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
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const tCmd = "test"

func TestConstantAttack(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder

	const dur = 50 * time.Millisecond
	const freq = 10_000 // req/s

	// Estimated: 500 requests -> dur*(freq/1000) (1s = 1000ms)
	// Tolerate 1% margin compared to estimate.
	const estimatedMinReq = 495
	const estimatedMaxReq = 505

	rcvdCount := new(int32)

	var countFn http.HandlerFunc = func(http.ResponseWriter, *http.Request) {
		atomic.AddInt32(rcvdCount, 1)
	}
	s := httptest.NewServer(countFn)

	err := run([]string{tCmd, "-u", s.URL, "-f", strconv.Itoa(freq), "-d", dur.String()}, &stdout, &stderr)
	if err != nil {
		t.Fatal("Unexpected error:", err)
	}

	s.Close()

	if *rcvdCount < estimatedMinReq || *rcvdCount > estimatedMaxReq {
		t.Errorf("Expected %d < requests < %d, got %d requests", estimatedMinReq, estimatedMaxReq, *rcvdCount)
	}

	if stderr.Len() != 0 {
		t.Error("Expected no output to stderr, got:\n" + stderr.String())
	}

	output := stdout.String()

	paceStr := fmt.Sprintf("Constant{%d hits/1s} for %s", freq, dur.String())
	if !strings.Contains(output, paceStr) {
		t.Error("Command didn't print expected pace. Log:\n" + stdout.String())
	}

	if !strings.Contains(output, "---- Report ----") {
		t.Fatal("Command didn't print results. Log:\n" + stdout.String())
	}

	rcvdCountStr := strconv.Itoa(int(*rcvdCount))
	if !strings.Contains(output, "Requests      [total, rate, throughput]         "+rcvdCountStr) {
		t.Error("Reported requests number doesn't match number of received events ("+rcvdCountStr+").",
			"Log:\n"+stdout.String())
	}

	if !strings.Contains(output, "Success       [ratio]                           100.00%") {
		t.Error("Expected a reported success of 100%. Log:\n" + stdout.String())
	}
}

func TestRampAttack(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder

	const dur = 50 * time.Millisecond // 5 intervals of 10ms
	const freq = 5_000                // req/s

	// Estimated: 150 requests -> 5 increments of (dur/5)*((freq/5)/1000) (1s = 1000ms)
	// Tolerate 1% margin compared to estimate.
	const estimatedMinReq = 148
	const estimatedMaxReq = 152

	rcvdCount := new(int32)

	var countFn http.HandlerFunc = func(http.ResponseWriter, *http.Request) {
		atomic.AddInt32(rcvdCount, 1)
	}
	s := httptest.NewServer(countFn)

	err := run([]string{tCmd, "-u", s.URL, "-f", strconv.Itoa(freq), "-d", dur.String(),
		"-m=ramp"},
		&stdout, &stderr)
	if err != nil {
		t.Fatal("Unexpected error:", err)
	}

	s.Close()

	if *rcvdCount < estimatedMinReq || *rcvdCount > estimatedMaxReq {
		t.Errorf("Expected %d < requests < %d, got %d requests", estimatedMinReq, estimatedMaxReq, *rcvdCount)
	}

	if stderr.Len() != 0 {
		t.Error("Expected no output to stderr, got:\n" + stderr.String())
	}

	output := stdout.String()

	paceStr := fmt.Sprintf("Ramp{5 intervals, %d hits/1s increments} for %s", freq/5, dur.String())
	if !strings.Contains(output, paceStr) {
		t.Error("Command didn't print expected pace. Log:\n" + stdout.String())
	}

	if !strings.Contains(output, "---- Report ----") {
		t.Fatal("Command didn't print results. Log:\n" + stdout.String())
	}

	rcvdCountStr := strconv.Itoa(int(*rcvdCount))
	if !strings.Contains(output, "Requests      [total, rate, throughput]         "+rcvdCountStr) {
		t.Error("Reported requests number doesn't match number of received events ("+rcvdCountStr+").",
			"Log:\n"+stdout.String())
	}

	if !strings.Contains(output, "Success       [ratio]                           100.00%") {
		t.Error("Expected a reported success of 100%. Log:\n" + stdout.String())
	}
}

func TestTimeout(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder

	const dur = 10 * time.Millisecond
	const freq = 100 // send only 1 request (100/s * 0.01s)

	const timeout = 1 * time.Millisecond

	var handleSlowFn http.HandlerFunc = func(http.ResponseWriter, *http.Request) {
		time.Sleep(timeout * 3)
	}
	s := httptest.NewServer(handleSlowFn)

	err := run([]string{tCmd, "-u", s.URL, "-f", strconv.Itoa(freq), "-d", dur.String(),
		"-t", timeout.String()},
		&stdout, &stderr,
	)
	if err != nil {
		t.Fatal("Unexpected error:", err)
	}

	s.Close()

	if stderr.Len() != 0 {
		t.Error("Expected no output to stderr, got:\n" + stderr.String())
	}

	output := stdout.String()

	if !strings.Contains(output, "---- Report ----") {
		t.Fatal("Command didn't print results. Log:\n" + stdout.String())
	}

	if !strings.Contains(output, "Status Codes  [code:count]                      0:1") {
		t.Error("Expected request to be reported as failed. Log:\n" + stdout.String())
	}

	if !strings.Contains(output, "Client.Timeout exceeded while awaiting headers") {
		t.Error("Expected request to time out. Log:\n" + stdout.String())
	}
}

func TestOutput(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder

	const dur = 10 * time.Millisecond
	const freq = 100 // send only 1 request (100/s * 0.01s)

	tmpFile, err := ioutil.TempFile("", "attackr")
	if err != nil {
		t.Fatal("Error creating temp output file:", err)
	}
	_ = tmpFile.Close()

	t.Cleanup(func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			t.Fatal("Failed to remove temp output file:", err)
		}
	})

	var handleFn http.HandlerFunc = func(http.ResponseWriter, *http.Request) {}
	s := httptest.NewServer(handleFn)

	err = run([]string{tCmd, "-u", s.URL, "-f", strconv.Itoa(freq), "-d", dur.String(),
		"-o", tmpFile.Name()},
		&stdout, &stderr,
	)
	if err != nil {
		t.Fatal("Unexpected error:", err)
	}

	s.Close()

	if stderr.Len() != 0 {
		t.Error("Expected no output to stderr, got:\n" + stderr.String())
	}

	f, err := os.Open(tmpFile.Name())
	if err != nil {
		t.Fatal("Failed to open temp output file:", err)
	}

	fs, err := f.Stat()
	if err != nil {
		t.Fatal("Failed to read info of temp output file:", err)
	}

	if fs.Size() == 0 {
		t.Error("Expected temp output file", f.Name(), "to contain results")
	}
}

func TestArgs(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder

	t.Run("missing -u flag", func(t *testing.T) {
		err := run([]string{tCmd}, &stdout, &stderr)
		if err == nil {
			t.Fatal("Expected command to fail")
		}

		expectMsg := "target URL isn't set"
		if errStr := err.Error(); !strings.Contains(errStr, expectMsg) {
			t.Fatalf("Unexpected error message: %q", errStr)
		}
	})

	t.Run("invalid -u value", func(t *testing.T) {
		err := run([]string{tCmd, "-u", "://invalid"}, &stdout, &stderr)
		if err == nil {
			t.Fatal("Expected command to fail")
		}

		expectMsg := "invalid target URL"
		if errStr := err.Error(); !strings.Contains(errStr, expectMsg) {
			t.Fatalf("Unexpected error message: %q", errStr)
		}
	})

	t.Run("unsupported -m value", func(t *testing.T) {
		const mode = "test"

		err := run([]string{tCmd, "-u=http://target", "-m", mode}, &stdout, &stderr)
		if err == nil {
			t.Fatal("Expected command to fail")
		}

		expectMsg := `unsupported mode "` + mode + `"`
		if errStr := err.Error(); !strings.Contains(errStr, expectMsg) {
			t.Fatalf("Unexpected error message: %q", errStr)
		}
	})

	t.Run("value of -f exceeds limit", func(t *testing.T) {
		aboveLimit := strconv.FormatUint(math.MaxInt32+1, 10)

		err := run([]string{tCmd, "-u=http://target", "-f", aboveLimit}, &stdout, &stderr)
		if err == nil {
			t.Fatal("Expected command to fail")
		}

		expectMsg := "frequency " + aboveLimit + " overflows the capacity of an integer"
		if errStr := err.Error(); !strings.Contains(errStr, expectMsg) {
			t.Fatalf("Unexpected error message: %q", errStr)
		}
	})

	t.Run("value of -s exceeds limit", func(t *testing.T) {
		aboveLimit := strconv.FormatUint(uint64(maxMsgSizeBytes)+1, 10)

		err := run([]string{tCmd, "-u=http://target", "-s", aboveLimit}, &stdout, &stderr)
		if err == nil {
			t.Fatal("Expected command to fail")
		}

		expectMsg := "message size " + aboveLimit + " B exceeds the maximum"
		if errStr := err.Error(); !strings.Contains(errStr, expectMsg) {
			t.Fatalf("Unexpected error message: %q", errStr)
		}
	})
}
