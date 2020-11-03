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

// Package recorder provides interfaces to record CloudEvents.
package recorder

import (
	"context"
	"errors"
	"sync"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
)

// DefaultStoreSize is the size to pre-allocate to the events storage when not
// explicitly defined.
const DefaultStoreSize = 1000

// Size of the buffered channel received events are sent to before being
// processed.
const receiveBufferSize = 100

// EventStore is a store of timestamps of received events keyed by event ID.
type EventStore map[ /*event id*/ string] /*rcv time*/ time.Time

// EventRecorder can record individual events into an event store and return
// the events it has recorded.
type EventRecorder interface {
	// Run runs the event recorder.
	Run(context.Context) error
	// Record records an event into the event store.
	Record(cloudevents.Event)
	// Recorded returns the events recorded so far.
	Recorded() EventStore
}

var _ EventRecorder = (*AsyncEventRecorder)(nil)

// AsyncEventRecorder is an EventRecorder that processes events asynchronously,
// without blocking the caller.
type AsyncEventRecorder struct {
	receivedCh chan *recordedEvent

	sync.RWMutex
	recordedEvents EventStore
}

// NewAsyncEventRecorder returns a new AsyncEventRecorder.
func NewAsyncEventRecorder(storeSize uint) *AsyncEventRecorder {
	if storeSize == 0 {
		storeSize = DefaultStoreSize
	}

	return &AsyncEventRecorder{
		receivedCh:     make(chan *recordedEvent, receiveBufferSize),
		recordedEvents: make(EventStore, storeSize),
	}
}

// recordedEvent is an intermediate structure that contains the details of an
// event that needs to be recorded.
type recordedEvent struct {
	id    string
	rcvAt time.Time
}

// Run implements EventRecorder.
func (r *AsyncEventRecorder) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil

		case e := <-r.receivedCh:
			if _, exists := r.recordedEvents[e.id]; exists {
				return errors.New("received duplicate event ID " + e.id)
			}

			r.Lock()
			r.recordedEvents[e.id] = e.rcvAt
			r.Unlock()
		}
	}
}

// Record implements EventRecorder.
func (r *AsyncEventRecorder) Record(e cloudevents.Event) {
	r.receivedCh <- &recordedEvent{
		id:    e.ID(),
		rcvAt: time.Now(),
	}
}

// Recorded implements EventRecorder.
func (r *AsyncEventRecorder) Recorded() EventStore {
	r.RLock()
	defer r.RUnlock()

	return r.recordedEvents
}

// QueueProfiler wraps the QueueLength method.
type QueueProfiler interface {
	// QueueLength returns the current length of a recorder's receive queue.
	QueueLength() int
}

var _ QueueProfiler = (*AsyncEventRecorder)(nil)

// QueueLength implements QueueProfiler.
// This call is not thread-safe because multiple writers send concurrently to
// the queue while we read its length, but the returned value should be
// accurate enough to get a rough estimate of how the system is coping with the
// flow.
// https://groups.google.com/g/golang-nuts/c/yQw1Wx6BoUU/m/7z83a1MZEAAJ
func (r *AsyncEventRecorder) QueueLength() int {
	return len(r.receivedCh)
}
