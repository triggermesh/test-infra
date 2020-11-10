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
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	uuid "github.com/rogpeppe/fastuuid"
	vegeta "github.com/tsenart/vegeta/v12/lib"
)

const (
	defaultFrequency = 1_000 // events/s

	defaultMsgSizeBytes = 2 * 1024  // 2 KiB
	maxMsgSizeBytes     = 32 * 1024 // 32 KiB

	defaultAttackDuration = 10 * time.Second
	defaultClientTimeout  = 10 * time.Second

	rampAttackIntervals = 5

	ceType   = "io.triggermesh.perf.drill"
	ceSource = "attackr"
)

func main() {
	if err := run(os.Args, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "Error running command: %s\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	cmdName := filepath.Base(args[0])

	flags := flag.NewFlagSet(cmdName, flag.ExitOnError)
	flags.SetOutput(stderr)

	opts, err := readOpts(flags, args)
	if err != nil {
		return fmt.Errorf("reading options: %w", err)
	}

	var enc vegeta.Encoder
	if *opts.output != "" {
		output, err := os.Create(*opts.output)
		if err != nil {
			return fmt.Errorf("creating output file %q: %w", *opts.output, err)
		}
		defer output.Close()

		enc = vegeta.NewEncoder(output)
	}

	va := vegeta.NewAttacker(
		vegeta.Workers(*opts.workers),
		vegeta.Timeout(*opts.timeout),
	)

	trg := cloudeventTargeter(opts)

	var atk Attacker

	switch *opts.mode {
	case modeConstant:
		atk = NewConstantAttacker(trg, va, *opts.frequency)
	case modeRamp:
		atk, err = NewRampAttacker(trg, va, *opts.frequency, rampAttackIntervals)
		if err != nil {
			return fmt.Errorf("creating ramp attacker: %w", err)
		}
	default:
		return fmt.Errorf("mode %q is unimplemented", *opts.mode)
	}

	fmt.Fprintln(stdout, "Running attack", atk, "for", *opts.duration)

	var metrics vegeta.Metrics

	for res := range atk.Attack(*opts.duration) {
		metrics.Add(res)
		if enc != nil {
			enc(res)
		}
	}

	metrics.Close()

	fmt.Fprintln(stdout, "Attack completed")

	fmt.Fprintln(stdout, "---- Report ----")

	if err := vegeta.NewTextReporter(&metrics).Report(stdout); err != nil {
		return fmt.Errorf("writing attack report: %w", err)
	}

	return nil
}

// cmdOpts are the options that can be passed to the command.
type cmdOpts struct {
	targetURL *string
	mode      *mode
	frequency *uint
	msgSize   *uint
	duration  *time.Duration
	timeout   *time.Duration
	workers   *uint64
	output    *string
}

// readOpts parses and validates options from commmand-line flags.
func readOpts(f *flag.FlagSet, args []string) (*cmdOpts, error) {
	opts := &cmdOpts{}

	opts.targetURL = f.String("u", "", "URL of the CloudEvents receiver to send events to")
	modeStr := f.String("m", modeConstant.String(), "Mode of operation")
	opts.frequency = f.Uint("f", defaultFrequency, "Frequency of requests in events/s")
	opts.msgSize = f.Uint("s", defaultMsgSizeBytes, "Size of the events' data in bytes")
	opts.duration = f.Duration("d", defaultAttackDuration, "Duration of the attack")
	opts.timeout = f.Duration("t", defaultClientTimeout, "Maximum time to wait for each request to be responded to")
	opts.workers = f.Uint64("w", vegeta.DefaultWorkers, "Number of initial vegeta workers")
	opts.output = f.String("o", "", "File to write vegeta's binary results to, if defined")

	if err := f.Parse(args[1:]); err != nil {
		return nil, err
	}

	if *opts.targetURL == "" {
		return nil, fmt.Errorf("target URL isn't set")
	}
	if _, err := url.Parse(*opts.targetURL); err != nil {
		return nil, fmt.Errorf("invalid target URL: %w", err)
	}

	m, err := toMode(*modeStr)
	if err != nil {
		return nil, err
	}
	opts.mode = m

	if f := *opts.frequency; f > math.MaxInt32 {
		return nil, fmt.Errorf("frequency %d overflows the capacity of an integer", f)
	}

	if s := *opts.msgSize; s > maxMsgSizeBytes {
		return nil, fmt.Errorf("message size %d B exceeds the maximum of %d B", s, maxMsgSizeBytes)
	}

	if *opts.output != "" {
		parsedOutput := *opts.output
		if *opts.output, err = filepath.Abs(*opts.output); err != nil {
			return nil, fmt.Errorf("converting %q to an absolute path: %w", parsedOutput, err)
		}
	}

	return opts, nil
}

// cloudeventTargeter returns a Targeter that generates CloudEvents with static
// data and IDs that are guaranteed to be unique.
func cloudeventTargeter(o *cmdOpts) vegeta.Targeter {
	uuidGen := uuid.MustNewGenerator()
	data := []byte(strings.Repeat("0", int(*o.msgSize)))

	return func(t *vegeta.Target) error {
		t.Method = http.MethodPost
		t.URL = *o.targetURL

		// we avoid using http.Header.Set() because it attempts to
		// sanitize every input, making it more expensive than
		// accessing the Header map directly.
		t.Header = http.Header{
			"Ce-Id":          []string{uuidGen.Hex128()},
			"Ce-Type":        []string{ceType},
			"Ce-Source":      []string{ceSource},
			"Ce-Specversion": []string{"1.0"},
			"Content-Type":   []string{"text/plain"},
		}

		t.Body = data

		return nil
	}
}
