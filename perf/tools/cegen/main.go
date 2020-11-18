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
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/mailru/easyjson/buffer"
	jwriter "github.com/mailru/easyjson/jwriter"
	uuid "github.com/rogpeppe/fastuuid"
	"github.com/sethvargo/go-signalcontext"
)

const (
	ceType   = "io.triggermesh.perf.drill"
	ceSource = "cegen"
)

func main() {
	ctx, cancel := signalcontext.On(syscall.SIGINT, syscall.SIGTERM, syscall.SIGPIPE)
	defer cancel()

	if err := run(ctx, os.Args, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "Error running command: %s\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	cmdName := filepath.Base(args[0])

	flags := flag.NewFlagSet(cmdName, flag.ExitOnError)
	flags.SetOutput(stderr)

	opts, err := readOpts(flags, args)
	if err != nil {
		return fmt.Errorf("reading options: %w", err)
	}

	data := []byte(*opts.ceData)

	if strings.HasPrefix(*opts.ceData, "@") {
		absPath, err := filepath.Abs(strings.TrimPrefix(*opts.ceData, "@"))
		if err != nil {
			return fmt.Errorf("converting %q to an absolute path: %w", *opts.ceData, err)
		}

		data, err = ioutil.ReadFile(absPath)
		if err != nil {
			return fmt.Errorf("reading data from file: %w", err)
		}
	}

	gen := NewCloudEventTargetsGenerator(*opts.targetURL, *opts.ceType, *opts.ceSource, data)

	for {
		select {
		case <-ctx.Done():
			return nil

		default:
			trg, err := gen.Generate()
			if err != nil {
				return fmt.Errorf("generating vegeta JSON target: %w", err)
			}

			fprintln(stdout, string(trg))
		}
	}
}

var fprintln = fmt.Fprintln

// cmdOpts are the options that can be passed to the command.
type cmdOpts struct {
	targetURL *string
	ceType    *string
	ceSource  *string
	ceData    *string
}

// readOpts parses and validates options from commmand-line flags.
func readOpts(f *flag.FlagSet, args []string) (*cmdOpts, error) {
	opts := &cmdOpts{}

	opts.targetURL = f.String("u", "", "URL of the CloudEvents receiver to use in generated vegeta targets")
	opts.ceType = f.String("t", ceType, "Value to set as the CloudEvent type context attribute")
	opts.ceSource = f.String("s", ceSource, "Value to set as the CloudEvent source context attribute")
	opts.ceData = f.String("d", "", "Data to set in generated CloudEvents. Prefix with '@' to read from a file")

	if err := f.Parse(args[1:]); err != nil {
		return nil, err
	}

	if *opts.targetURL == "" {
		return nil, fmt.Errorf("target URL isn't set")
	}
	if _, err := url.Parse(*opts.targetURL); err != nil {
		return nil, fmt.Errorf("invalid target URL: %w", err)
	}

	if *opts.ceData == "" {
		return nil, fmt.Errorf("event data isn't set")
	}

	return opts, nil
}

// CloudEventTargetsGenerator generates CloudEvent vegeta targets.
type CloudEventTargetsGenerator struct {
	targetURL string

	uuidGen    *uuid.Generator
	typeAttr   string
	sourceAttr string
	data       []byte
}

// NewCloudEventTargetsGenerator returns a generator that yields vegeta JSON
// targets containing CloudEvents with static data and IDs that are guaranteed
// to be unique.
func NewCloudEventTargetsGenerator(url, typeAttr, sourceAttr string, data []byte) *CloudEventTargetsGenerator {
	return &CloudEventTargetsGenerator{
		targetURL:  url,
		uuidGen:    uuid.MustNewGenerator(),
		typeAttr:   typeAttr,
		sourceAttr: sourceAttr,
		data:       data,
	}
}

// Buffer pool for jwriter.Writer's underlying Buffer and output.
var writerBufPool *sync.Pool

// Once used to initialize the buffer pool on the first call to Generate.
var bufOnce sync.Once

// Generate returns a target serialized as JSON.
func (g *CloudEventTargetsGenerator) Generate() ([]byte, error) {
	var t jsonTarget

	t.Method = http.MethodPost
	t.URL = g.targetURL

	// we avoid using http.Header.Set() because it attempts to
	// sanitize every input, making it more expensive than
	// accessing the Header map directly.
	t.Header = http.Header{
		"Ce-Id":          []string{g.uuidGen.Hex128()},
		"Ce-Type":        []string{g.typeAttr},
		"Ce-Source":      []string{g.sourceAttr},
		"Ce-Specversion": []string{"1.0"},
		"Content-Type":   []string{"application/json"},
	}

	t.Body = g.data

	// encode inside the Once fn to determine the size of buffers in sync pools
	bufOnce.Do(func() {
		var jw jwriter.Writer
		t.encode(&jw)
		dataBytes, _ := jw.BuildBytes()
		dataSize := len(dataBytes)

		writerBufPool = &sync.Pool{
			New: func() interface{} {
				return make([]byte, 0, dataSize)
			},
		}
	})

	writerBuf := writerBufPool.Get().([]byte)
	buildBuf := writerBufPool.Get().([]byte)
	defer writerBufPool.Put(buildBuf[:0])
	defer writerBufPool.Put(writerBuf[:0])

	jw := &jwriter.Writer{
		Buffer: buffer.Buffer{
			Buf: writerBuf,
		},
	}

	t.encode(jw)

	return jw.BuildBytes(buildBuf)
}
