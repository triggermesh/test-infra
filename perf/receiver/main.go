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
	"io"
	"log"
	"net/http"
	"os"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	cehttp "github.com/cloudevents/sdk-go/v2/protocol/http"
	"github.com/sethvargo/go-signalcontext"
)

const idleConnTimeout = 30 * time.Second

func main() {
	if err := run(os.Args, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "Error running command: %s\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	ctx, cancel := signalcontext.OnInterrupt()
	defer cancel()

	cli, err := cloudeventsClient()
	if err != nil {
		return fmt.Errorf("creating CloudEvents client: %w", err)
	}

	log.Print("Running CloudEvents handler")
	if err := cli.StartReceiver(ctx, func() {}); err != nil {
		return fmt.Errorf("during runtime of CloudEvents receiver: %w", err)
	}

	return nil
}

// cloudeventsClient returns a CloudEvents Client with sane defaults.
// In comparison with the client returned by cloudevents.NewDefaultClient, this
// client doesn't enable tracing and offers a configurable timeout for idle
// connections.
func cloudeventsClient() (cloudevents.Client, error) {
	t := http.DefaultTransport.(*http.Transport)
	t.IdleConnTimeout = idleConnTimeout

	p, err := cehttp.New(cehttp.WithRoundTripper(t))
	if err != nil {
		return nil, fmt.Errorf("creating cehttp.Protocol: %w", err)
	}

	return cloudevents.NewClient(p)
}
