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
	"sort"
	"time"

	"github.com/google/mako/go/quickstore"
	"knative.dev/pkg/test/mako"

	"thrpt-receiver/recorder"
)

// processResults returns the data from the given EventStore in a shape that
// can be published to Mako.
func processResults(s recorder.EventStore) []time.Time {
	return eventsToSortedTimestampsSlice(s)
}

// eventsToSortedTimestampsSlice returns a sorted slice of the timestamps of
// all events contained in the given EventStore.
func eventsToSortedTimestampsSlice(s recorder.EventStore) []time.Time {
	timestamps := make([]time.Time, 0, len(s))

	for _, ts := range s {
		timestamps = append(timestamps, ts)
	}

	sort.Slice(timestamps, func(x, y int) bool {
		return timestamps[x].Before(timestamps[y])
	})

	return timestamps
}

// publishThroughput calculates the received throughput based on the given
// timestamps, and publishes sample points to Mako.
func publishThroughput(q *quickstore.Quickstore, timestamps []time.Time) error {
	if len(timestamps) == 1 {
		return q.AddSamplePoint(
			mako.XTime(timestamps[0]),
			map[string]float64{makoKeyReceiveThroughput: 1},
		)
	}

	var i, thpt int

	for j, t := range timestamps[1:] {
		thpt++

		for i < j && t.Sub(timestamps[i]) > time.Second {
			i++
			thpt--
		}

		err := q.AddSamplePoint(
			mako.XTime(t),
			map[string]float64{makoKeyReceiveThroughput: float64(thpt)},
		)
		if err != nil {
			return err
		}
	}

	return nil
}
