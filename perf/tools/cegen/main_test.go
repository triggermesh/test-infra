/*
Copyright 2020 TriggerMesh Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"
)

const testTimeout = 1 * time.Second

const tCmd = "test"

func TestRun(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	origFprintln := fprintln
	defer func() {
		fprintln = origFprintln
	}()

	genCh := make(chan struct{})
	defer close(genCh)

	// Keep the executions of the main loop under control of the test logic
	// with a latch to prevent the CPU scheduler's randomness from making
	// this test flaky.
	fprintln = func(w io.Writer, a ...interface{}) (n int, err error) {
		genCh <- struct{}{}
		n, err = origFprintln(w, a...)
		genCh <- struct{}{}
		return
	}

	errCh := make(chan error)
	defer close(errCh)

	go func() {
		errCh <- run(ctx, []string{tCmd, "-u=http://target", "-d={}"}, &stdout, &stderr)
	}()

	// allow the loop to generate several targets in a row
	for i := 0; i < 4; i++ {
		select {
		case <-ctx.Done():
			t.Fatal("Test context marked as done:", ctx.Err())

		case err := <-errCh:
			t.Fatal("Command returned unexpectedly:", err)

		default:
			// allow one loop to complete
			<-genCh
			<-genCh
		}
	}

	// Let the loop enter the default "select" case one more time, then
	// cancel the context so the next iteration is guaranteed to hit the
	// ctx.Done() case.
	<-genCh
	cancel()
	<-genCh

	if err := <-errCh; err != nil {
		t.Fatal("Unexpected runtime error:", err)
	}

	output := stdout.String()
	r := bufio.NewReader(strings.NewReader(output))

	const expectTargets = 5 // 4 in a row + 1 in the latched call to cancel()

	targets := make([][]byte, 0, expectTargets)

	for {
		trg, err := r.ReadBytes('\n')
		if err == io.EOF {
			// we don't expect any byte after the final new line
			if len(trg) > 0 {
				t.Error("Found bytes after final newline:", string(trg))
			}
			break
		}
		if err != nil {
			t.Fatal("Reading from stdout:", err)
		}

		targets = append(targets, trg)
	}

	if l := len(targets); l != expectTargets {
		t.Errorf("Expected %d targets, got %d:\n%s", expectTargets, l, output)
	}
}

func TestDataFromFile(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	origFprintln := fprintln
	defer func() {
		fprintln = origFprintln
	}()

	genCh := make(chan struct{})
	defer close(genCh)

	// Keep the executions of the main loop under control of the test logic
	// with a latch to prevent the CPU scheduler's randomness from making
	// this test flaky.
	fprintln = func(w io.Writer, a ...interface{}) (n int, err error) {
		genCh <- struct{}{}
		n, err = origFprintln(w, a...)
		genCh <- struct{}{}
		return
	}

	testData := []byte(`{"msg":"hello, world!"}`)
	testDataBase64 := []byte("eyJtc2ciOiJoZWxsbywgd29ybGQhIn0=")

	tmpFile, err := ioutil.TempFile("", "cegen")
	if err != nil {
		t.Fatal("Error creating temp output file:", err)
	}
	if _, err := tmpFile.Write(testData); err != nil {
		t.Fatal("Error writing test data to temp file:", err)
	}
	_ = tmpFile.Close()

	t.Cleanup(func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			t.Fatal("Failed to remove temp data file:", err)
		}
	})

	errCh := make(chan error)
	defer close(errCh)

	go func() {
		errCh <- run(ctx, []string{tCmd, "-u=http://target", "-d=@" + tmpFile.Name()}, &stdout, &stderr)
	}()

	select {
	case <-ctx.Done():
		t.Fatal("Test context marked as done:", ctx.Err())

	case err := <-errCh:
		t.Fatal("Command returned unexpectedly:", err)

	default:
		// Let the loop enter the default "select" case once,
		// then cancel the context so the next iteration is
		// guaranteed to hit the ctx.Done() case.
		<-genCh
		cancel()
		<-genCh
	}

	if err := <-errCh; err != nil {
		t.Fatal("Unexpected runtime error:", err)
	}

	output := stdout.String()
	r := bufio.NewReader(strings.NewReader(output))

	trg, err := r.ReadBytes('\n')
	if err != nil {
		t.Fatal("Reading from stdout:", err)
	}

	if !bytes.Contains(trg, testDataBase64) {
		t.Error("Expected target to contain base64-encoded test data:\n" + output)
	}
}

func TestArgs(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	t.Run("missing -u flag", func(t *testing.T) {
		err := run(ctx, []string{tCmd}, &stdout, &stderr)
		if err == nil {
			t.Fatal("Expected command to fail")
		}

		expectMsg := "target URL isn't set"
		if errStr := err.Error(); !strings.Contains(errStr, expectMsg) {
			t.Fatalf("Unexpected error message: %q", errStr)
		}
	})

	t.Run("invalid -u value", func(t *testing.T) {
		err := run(ctx, []string{tCmd, "-u", "://invalid"}, &stdout, &stderr)
		if err == nil {
			t.Fatal("Expected command to fail")
		}

		expectMsg := "invalid target URL"
		if errStr := err.Error(); !strings.Contains(errStr, expectMsg) {
			t.Fatalf("Unexpected error message: %q", errStr)
		}
	})

	t.Run("empty -d value", func(t *testing.T) {
		err := run(ctx, []string{tCmd, "-u", "http://target"}, &stdout, &stderr)
		if err == nil {
			t.Fatal("Expected command to fail")
		}

		expectMsg := "event data isn't set"
		if errStr := err.Error(); !strings.Contains(errStr, expectMsg) {
			t.Fatalf("Unexpected error message: %q", errStr)
		}
	})
}

func BenchmarkGenerate(b *testing.B) {
	const (
		url = "http://localhost"
		typ = "test.event"
		src = "cegen/go/benchmark"
	)

	data := bytes.Repeat([]byte{'0'}, 2048)

	g := NewCloudEventTargetsGenerator(url, typ, src, data)

	for i := 0; i < b.N; i++ {
		if _, err := g.Generate(); err != nil {
			b.Error("Generate returned an error:", err)
		}
	}
}
