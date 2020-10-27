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
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/aws-sdk-go/service/sqs/sqsiface"
)

const tCmd = "test"

func TestSend(t *testing.T) {
	cli := &mockSQSSender{}

	var stdout strings.Builder
	var stderr strings.Builder

	numMsg := 9_999   // some value below maxNumMsgs
	expectReq := 1250 // assuming msgBatchSize is 8

	err := run(cli, []string{tCmd, "-u=http://queue", "-n", strconv.Itoa(numMsg)}, &stdout, &stderr)
	if err != nil {
		t.Fatal("Unexpected error: ", err)
	}

	gotReq := cli.reqSent
	if gotReq != expectReq {
		t.Errorf("Expected %d requests to be sent, got %d", expectReq, gotReq)
	}

	gotMsg := cli.msgsSent
	if gotMsg != numMsg {
		t.Errorf("Expected %d messages to be sent, got %d", numMsg, gotMsg)
	}
}

func TestArgs(t *testing.T) {
	cli := &mockSQSSender{}

	var stdout strings.Builder
	var stderr strings.Builder

	t.Run("missing -u flag", func(t *testing.T) {
		err := run(cli, []string{tCmd}, &stdout, &stderr)
		if err == nil {
			t.Fatal("Expected command to fail")
		}

		expectMsg := "queue URL isn't set"
		if errStr := err.Error(); !strings.Contains(errStr, expectMsg) {
			t.Fatalf("Unexpected error message: %q", errStr)
		}
	})

	t.Run("invalid -u value", func(t *testing.T) {
		err := run(cli, []string{tCmd, "-u", "://invalid"}, &stdout, &stderr)
		if err == nil {
			t.Fatal("Expected command to fail")
		}

		expectMsg := "invalid queue URL"
		if errStr := err.Error(); !strings.Contains(errStr, expectMsg) {
			t.Fatalf("Unexpected error message: %q", errStr)
		}
	})

	t.Run("value of -n exceeds limit", func(t *testing.T) {
		aboveLimit := strconv.FormatUint(uint64(maxNumMsgs)+1, 10)

		err := run(cli, []string{tCmd, "-u=http://queue", "-n", aboveLimit}, &stdout, &stderr)
		if err == nil {
			t.Fatal("Expected command to fail")
		}

		expectMsg := "number of messages " + aboveLimit + " exceeds the maximum"
		if errStr := err.Error(); !strings.Contains(errStr, expectMsg) {
			t.Fatalf("Unexpected error message: %q", errStr)
		}
	})

	t.Run("value of -s exceeds limit", func(t *testing.T) {
		aboveLimit := strconv.FormatUint(uint64(maxMsgSizeBytes)+1, 10)

		err := run(cli, []string{tCmd, "-u=http://queue", "-s", aboveLimit}, &stdout, &stderr)
		if err == nil {
			t.Fatal("Expected command to fail")
		}

		expectMsg := "message size " + aboveLimit + " B exceeds the maximum"
		if errStr := err.Error(); !strings.Contains(errStr, expectMsg) {
			t.Fatalf("Unexpected error message: %q", errStr)
		}
	})
}

type mockSQSSender struct {
	sqsiface.SQSAPI

	sync.Mutex
	reqSent  int
	msgsSent int
}

func (m *mockSQSSender) SendMessageBatch(in *sqs.SendMessageBatchInput) (*sqs.SendMessageBatchOutput, error) {
	if in != nil {
		m.Lock()
		m.reqSent++
		m.msgsSent += len(in.Entries)
		m.Unlock()
	}

	return nil, nil
}
