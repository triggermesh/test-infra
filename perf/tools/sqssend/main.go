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
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/aws-sdk-go/service/sqs/sqsiface"
)

const (
	// https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-batch-api-actions.html
	maxBatchSizeBytes = 256 * 1024 // 256 KiB
	msgBatchSize      = 8

	defaultMsgSizeBytes = 2 * 1024 // 2 KiB
	defaultNumMsgs      = 100

	maxMsgSizeBytes uint = maxBatchSizeBytes / msgBatchSize // 32 KiB
)

func main() {
	cg := &clientGetter{configProvider: session.Must(session.NewSession())}

	if err := run(cg, os.Args, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "Error running command: %s\n", err)
		os.Exit(1)
	}
}

func run(cg ClientGetter, args []string, stdout, stderr io.Writer) error {
	cmdName := filepath.Base(args[0])

	flags := flag.NewFlagSet(cmdName, flag.ExitOnError)
	flags.SetOutput(stderr)

	opts, err := readOpts(flags, args)
	if err != nil {
		return fmt.Errorf("reading options: %w", err)
	}

	cli := cg.Get(parseRegionFromQueueURL(opts.queueURL))

	return sendMsgBatches(cli, prepareMsgBatches(opts), stdout)
}

// cmdOpts are the options that can be passed to the command.
type cmdOpts struct {
	queueURL *url.URL
	numMsgs  *uint
	msgSize  *uint
}

// readOpts parses and validates options from commmand-line flags.
func readOpts(f *flag.FlagSet, args []string) (*cmdOpts, error) {
	opts := &cmdOpts{}
	queueURL := f.String("u", "", "URL of the Amazon SQS queue to send messages to")
	opts.numMsgs = f.Uint("n", defaultNumMsgs, "Number of messages to send")
	opts.msgSize = f.Uint("s", defaultMsgSizeBytes, "Size of the messages in bytes")

	err := f.Parse(args[1:])
	if err != nil {
		return nil, err
	}

	if *queueURL == "" {
		return nil, fmt.Errorf("queue URL isn't set")
	}
	if opts.queueURL, err = url.Parse(*queueURL); err != nil {
		return nil, fmt.Errorf("invalid queue URL: %w", err)
	}

	if s := *opts.msgSize; s > maxMsgSizeBytes {
		return nil, fmt.Errorf("message size %d B exceeds the maximum of %d B", s, maxMsgSizeBytes)
	}

	return opts, nil
}

var awsRegionRegexp = regexp.MustCompile(`[a-z]{2}(-gov)?-[a-z]+-\d`)

// parseRegionFromQueueURL reads the AWS region from the SQS queue's URL.
func parseRegionFromQueueURL(url *url.URL) (region *string) {
	// The expected host format is "sqs.us-west-2.amazonaws.com/123456789012/MyQueue"
	subs := strings.Split(url.Host, ".")

	if len(subs) == 4 && awsRegionRegexp.MatchString(subs[1]) {
		region = &subs[1]
	}

	return
}

// calculateNumBatches returns the appropriate number of batch requests to
// generate for the total number of messages to send.
func calculateNumBatches(numMsgs uint) uint {
	numBatches := numMsgs / msgBatchSize
	if numMsgs%msgBatchSize != 0 {
		numBatches++
	}

	return numBatches
}

// prepareMsgBatches builds a list of batch requests containing the messages to
// be sent to the queue.
func prepareMsgBatches(o *cmdOpts) []*sqs.SendMessageBatchInput {
	payload := strings.Repeat("0", int(*o.msgSize))
	queueURL := o.queueURL.String()

	batches := make([]*sqs.SendMessageBatchInput, 0, calculateNumBatches(*o.numMsgs))

	for i := uint(0); i < *o.numMsgs; i++ {
		if i%msgBatchSize == 0 {
			batches = append(batches, &sqs.SendMessageBatchInput{
				Entries:  make([]*sqs.SendMessageBatchRequestEntry, 0, msgBatchSize),
				QueueUrl: &queueURL,
			})
		}

		msg := &sqs.SendMessageBatchRequestEntry{
			Id:          aws.String(fmt.Sprintf("%05d", i)),
			MessageBody: &payload,
		}

		curEntries := &(batches[len(batches)-1].Entries)
		*curEntries = append(*curEntries, msg)
	}

	return batches
}

// sendMsgBatches sends the given message batches concurrently.
func sendMsgBatches(cli Client, batches []*sqs.SendMessageBatchInput, stdout io.Writer) error {
	errCh := make(chan error, len(batches))
	defer close(errCh)

	for i := range batches {
		go func(i int) {
			_, err := cli.SendMessageBatch(batches[i])
			errCh <- err
		}(i)
	}

	var errs []error

	for i := 0; i < cap(errCh); i++ {
		if err := <-errCh; err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("sending messages: %w", &errList{errs: errs})
	}

	return nil
}

// Client is an alias for sqsiface.SQSAPI.
type Client = sqsiface.SQSAPI

// ClientGetter can obtain SQS clients.
type ClientGetter interface {
	Get(region *string) Client
}

// clientGetter gets SQS clients using a awsclient.ConfigProvider.
type clientGetter struct {
	configProvider client.ConfigProvider
}

// clientGetter implements ClientGetter.
var _ ClientGetter = (*clientGetter)(nil)

// Get implements ClientGetter.
func (g *clientGetter) Get(region *string) Client {
	var cfgs []*aws.Config

	if region != nil {
		cfgs = append(cfgs, aws.NewConfig().WithRegion(*region))
	}

	return sqs.New(g.configProvider, cfgs...)
}

// ClientGetterFunc allows the use of ordinary functions as ClientGetter.
type ClientGetterFunc func(region *string) Client

// ClientGetterFunc implements ClientGetter.
var _ ClientGetter = (ClientGetterFunc)(nil)

// Get implements ClientGetter.
func (f ClientGetterFunc) Get(region *string) Client {
	return f(region)
}

type errList struct {
	errs []error
}

var _ error = (*errList)(nil)

// Error implements the error interface.
func (e *errList) Error() string {
	return fmt.Sprintf("%q", e.errs)
}
