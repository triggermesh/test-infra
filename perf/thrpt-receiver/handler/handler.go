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

// Package handler contains interfaces to process CloudEvents.
package handler

import (
	"context"

	cloudevents "github.com/cloudevents/sdk-go/v2"
)

// Handler runs a CloudEvents receiver.
type Handler struct {
	cli      cloudevents.Client
	recordFn RecordEventFunc
}

// RecordEventFunc is a function that records a given CloudEvent.
type RecordEventFunc func(cloudevents.Event)

// NewHandler returns a new Handler for the given CloudEvents client.
func NewHandler(cli cloudevents.Client, recordFn RecordEventFunc) *Handler {
	return &Handler{
		cli:      cli,
		recordFn: recordFn,
	}
}

// Run starts the handler and blocks until it returns.
func (h *Handler) Run(ctx context.Context) error {
	return h.cli.StartReceiver(ctx, h.receive)
}

// receive implements the handler's receive logic.
func (h *Handler) receive(e cloudevents.Event) {
	h.recordFn(e)
}
