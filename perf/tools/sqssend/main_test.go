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
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/aws-sdk-go/service/sqs/sqsiface"
)

const tCmd = "test"

func TestSend(t *testing.T) {
	testCases := []struct {
		numMsg    int
		expectReq int
	}{
		{
			numMsg:    0,
			expectReq: 0,
		},
		{
			numMsg:    1,
			expectReq: 1,
		},
		{
			numMsg:    msgBatchSize,
			expectReq: 1,
		},
		{
			numMsg:    msgBatchSize + 1,
			expectReq: 2,
		},
		{
			numMsg:    9_999,
			expectReq: 1250, // assuming msgBatchSize is 8
		},
	}

	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			cli := &mockSQSSender{}
			cg := staticClientGetter(cli)

			var stdout strings.Builder
			var stderr strings.Builder

			err := run(cg, []string{tCmd, "-u=http://queue", "-n", strconv.Itoa(tc.numMsg)}, &stdout, &stderr)
			if err != nil {
				t.Fatal("Unexpected error: ", err)
			}

			gotReq := cli.reqSent
			if gotReq != tc.expectReq {
				t.Errorf("Expected %d requests to be sent, got %d", tc.expectReq, gotReq)
			}

			gotMsg := cli.msgsSent
			if gotMsg != tc.numMsg {
				t.Errorf("Expected %d messages to be sent, got %d", tc.numMsg, gotMsg)
			}
		})
	}
}

func TestArgs(t *testing.T) {
	cli := &mockSQSSender{}
	cg := staticClientGetter(cli)

	var stdout strings.Builder
	var stderr strings.Builder

	t.Run("missing -u flag", func(t *testing.T) {
		err := run(cg, []string{tCmd}, &stdout, &stderr)
		if err == nil {
			t.Fatal("Expected command to fail")
		}

		expectMsg := "queue URL isn't set"
		if errStr := err.Error(); !strings.Contains(errStr, expectMsg) {
			t.Fatalf("Unexpected error message: %q", errStr)
		}
	})

	t.Run("invalid -u value", func(t *testing.T) {
		err := run(cg, []string{tCmd, "-u", "://invalid"}, &stdout, &stderr)
		if err == nil {
			t.Fatal("Expected command to fail")
		}

		expectMsg := "invalid queue URL"
		if errStr := err.Error(); !strings.Contains(errStr, expectMsg) {
			t.Fatalf("Unexpected error message: %q", errStr)
		}
	})

	t.Run("value of -s exceeds limit", func(t *testing.T) {
		aboveLimit := strconv.FormatUint(uint64(maxMsgSizeBytes)+1, 10)

		err := run(cg, []string{tCmd, "-u=http://queue", "-s", aboveLimit}, &stdout, &stderr)
		if err == nil {
			t.Fatal("Expected command to fail")
		}

		expectMsg := "message size " + aboveLimit + " B exceeds the maximum"
		if errStr := err.Error(); !strings.Contains(errStr, expectMsg) {
			t.Fatalf("Unexpected error message: %q", errStr)
		}
	})
}

func TestParseRegionFromQueueURL(t *testing.T) {
	testCases := []struct {
		input  *url.URL
		expect *string
	}{
		{
			input:  &url.URL{Host: "sqs.us-west-2.amazonaws.com/123456789012/MyQueue"},
			expect: aws.String("us-west-2"),
		},
		{
			input:  &url.URL{Host: "not-sqs.us-west-2.not-amazonaws.not-com/not-account-id/MyQueue"},
			expect: aws.String("us-west-2"),
		},
		{
			input:  &url.URL{Host: "sqs.not-region.notamazon.notcom/123456789012/MyQueue"},
			expect: nil,
		},
		{
			input:  &url.URL{Host: "example.com"},
			expect: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.input.String(), func(t *testing.T) {
			out := parseRegionFromQueueURL(tc.input)

			if out == tc.expect { // both nil, success
				return
			}
			if out == nil && tc.expect != nil {
				t.Fatal("Unexpected nil output")
			}
			if out != nil && tc.expect == nil {
				t.Fatal("Expected nil output, got", *out)
			}
			if *out != *tc.expect {
				t.Errorf("Expected %s, got %s", *tc.expect, *out)
			}
		})
	}
}

// staticClientGetter transforms the given client interface into a ClientGetter.
func staticClientGetter(cli Client) ClientGetterFunc {
	return func(*string) Client {
		return cli
	}
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
