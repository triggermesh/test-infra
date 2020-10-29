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
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/sethvargo/go-signalcontext"
	"knative.dev/pkg/test/mako"

	"thrpt-receiver/handler"
	"thrpt-receiver/recorder"
)

const (
	defaultRecheckPeriod           = 5 * time.Second
	defaultConsecutiveQuietPeriods = 2

	makoKeyReceiveThroughput = "rt"
)

func main() {
	if err := run(os.Args, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "Error running command: %s\n", err)
		os.Exit(1)
	}
}

// cmdOpts are the options that can be passed to the command.
type cmdOpts struct {
	recheckPeriod           *time.Duration
	consecutiveQuietPeriods *uint
	estimatedTotalEvents    *uint
}

func run(args []string, stdout, stderr io.Writer) error {
	cmdName := filepath.Base(args[0])

	flags := flag.NewFlagSet(cmdName, flag.ExitOnError)
	flags.SetOutput(stderr)

	opts, err := readOpts(flags, args)
	if err != nil {
		return fmt.Errorf("reading options: %w", err)
	}

	ctx, cancel := signalcontext.OnInterrupt()
	defer cancel()

	cli, err := cloudevents.NewDefaultClient()
	if err != nil {
		return fmt.Errorf("creating CloudEvents client: %w", err)
	}

	rec := recorder.NewAsyncEventRecorder(*opts.estimatedTotalEvents)

	h := handler.NewHandler(cli, rec.Record)

	// ShutDownFunc will fail if called after the context passed to
	// mako.Setup got cancelled, so we use Background instead of
	// OnInterrupt to keep control over the lifecycle of the Mako sidecar.
	makoCtx, makoCancel := context.WithCancel(context.Background())
	defer makoCancel()
	makoCli, err := mako.Setup(makoCtx)
	if err != nil {
		return fmt.Errorf("setting up Mako client: %w", err)
	}
	defer makoCli.ShutDownFunc(context.Background())

	rcvCtx, rcvCancel := context.WithCancel(ctx)
	defer rcvCancel()

	var wg sync.WaitGroup
	wg.Add(2)

	log.Print("Running event recorder")
	go runRecorder(rcvCtx, rec, wg.Done)

	log.Print("Running CloudEvents handler")
	go runHandler(rcvCtx, h, wg.Done)

	log.Print("Waiting for the first event to be received")
	eventRcvd := make(chan struct{})
	go func() {
		if waitForFirstEvent(ctx, rec) {
			eventRcvd <- struct{}{}
		}
	}()

	select {
	case <-ctx.Done(): // early container termination
		cancel()
		wg.Wait()
		return nil

	case <-eventRcvd:
	}

	close(eventRcvd)

	log.Printf("Event received, waiting until no more event is being recorded for %d consecutive periods of %s",
		*opts.consecutiveQuietPeriods, *opts.recheckPeriod)
	waitUntilNoMoreRecordedEvent(ctx, rec, *opts.recheckPeriod, *opts.consecutiveQuietPeriods)

	rcvCancel()
	wg.Wait()

	log.Print("Received events count: ", len(rec.Recorded()))

	log.Print("Processing data")
	res := processResults(rec.Recorded())

	log.Print("Publishing results to Mako")
	if err = publishThroughput(makoCli.Quickstore, res); err != nil {
		return fmt.Errorf("publishing results to Mako: %w", err)
	}
	if err := makoCli.StoreAndHandleResult(); err != nil {
		return fmt.Errorf("storing published values in Mako: %w", err)
	}

	return nil
}

// readOpts parses and validates options from commmand-line flags.
func readOpts(f *flag.FlagSet, args []string) (*cmdOpts, error) {
	opts := &cmdOpts{}

	opts.recheckPeriod = f.Duration("recheck-period", defaultRecheckPeriod,
		"Frequency at which the recording of new events is being checked.")

	opts.consecutiveQuietPeriods = f.Uint("consecutive-quiet-periods", defaultConsecutiveQuietPeriods,
		"Consecutive recheck-period after which data is aggregated if no new event has been recorded.")

	opts.estimatedTotalEvents = f.Uint("estimated-total-events", recorder.DefaultStoreSize,
		"Estimated total number of events to receive. Used to pre-allocate memory.")

	if err := f.Parse(args[1:]); err != nil {
		return nil, err
	}

	return opts, nil
}

// runRecorder runs the given EventRecorder.
func runRecorder(ctx context.Context, rec recorder.EventRecorder, doneFn func()) {
	defer doneFn()

	if err := rec.Run(ctx); err != nil {
		log.Panic("Failure during runtime of event recorder: ", err)
	}
	log.Print("Stopped event recorder")
}

// runHandler runs the given CloudEvents handler.
func runHandler(ctx context.Context, h *handler.Handler, doneFn func()) {
	defer doneFn()

	if err := h.Run(ctx); err != nil {
		log.Panic("Failure during runtime of CloudEvents handler: %w", err)
	}
	log.Print("Stopped CloudEvents handler")
}

// waitForFirstEvent polls the given EventRecorder untils at least one event
// has been recorded. Returns false when ctx gets cancelled before an event was
// received.
func waitForFirstEvent(ctx context.Context, rec recorder.EventRecorder) /*received*/ bool {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false

		case <-ticker.C:
			if len(rec.Recorded()) > 0 {
				return true
			}
		}
	}
}

// waitUntilNoMoreRecordedEvent polls the given EventRecorder until it stops
// observing new events for the configured number of consecutive recheck periods.
func waitUntilNoMoreRecordedEvent(ctx context.Context, rec recorder.EventRecorder,
	recheckPeriod time.Duration, maxQuietPeriods uint) {

	var consecutiveQuietPeriods uint
	lastEventCount := len(rec.Recorded())

	ticker := time.NewTicker(recheckPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			eventCount := len(rec.Recorded())

			if eventCount-lastEventCount > 0 {
				consecutiveQuietPeriods = 0
			} else {
				consecutiveQuietPeriods++
				log.Print("Observed ", consecutiveQuietPeriods, " period(s) without event")
			}

			if consecutiveQuietPeriods == maxQuietPeriods {
				return
			}

			lastEventCount = eventCount
		}
	}
}
