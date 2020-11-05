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
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const tCmd = "test"

func TestAttack(t *testing.T) {
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
		t.Fatal("Unexpected error: ", err)
	}

	s.Close()

	if *rcvdCount < estimatedMinReq || *rcvdCount > estimatedMaxReq {
		t.Errorf("Expected %d < requests < %d, got %d requests", estimatedMinReq, estimatedMaxReq, *rcvdCount)
	}

	if stderr.Len() != 0 {
		t.Error("Expected no output to stderr, got:\n" + stderr.String())
	}

	output := stdout.String()

	paceStr := fmt.Sprintf("{%d hits/1s} for %s", freq, dur.String())
	if !strings.Contains(output, paceStr) {
		t.Error("Command didn't print expected pace. Log:\n" + stdout.String())
	}

	if !strings.Contains(output, "---- Results ----") {
		t.Fatal("Command didn't print results. Log:\n" + stdout.String())
	}

	rcvdCountStr := strconv.Itoa(int(*rcvdCount))
	if !strings.Contains(output, "requests     : "+rcvdCountStr) {
		t.Error("Reported requests number doesn't match number of received events ("+rcvdCountStr+").",
			"Log:\n"+stdout.String())
	}

	if !strings.Contains(output, "success %    : 100") {
		t.Error("Expected a reported success of 100%. Log:\n" + stdout.String())
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
