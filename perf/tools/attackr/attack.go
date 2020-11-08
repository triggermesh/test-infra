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
	"math"
	"time"

	vegeta "github.com/tsenart/vegeta/v12/lib"
)

// Attacker is the interface implemented by custom vegeta Attackers.
type Attacker interface {
	// Attack performs an attack on a target.
	Attack(time.Duration) vegeta.Metrics
	// The embedded Stringer interface allows to return a pretty-printed
	// description of the attack.
	fmt.Stringer
}

// genericAttacker wraps fields included in all custom vegeta Attackers.
type genericAttacker struct {
	trg vegeta.Targeter
	atk *vegeta.Attacker
}

// ConstantAttacker runs an attack at a constant rate.
type ConstantAttacker struct {
	genericAttacker
	rate vegeta.ConstantPacer
}

var _ Attacker = (*ConstantAttacker)(nil)

// NewConstantAttacker returns a new ConstantAttacker configured to attack at
// the given frequency (in events/sec).
func NewConstantAttacker(trg vegeta.Targeter, atk *vegeta.Attacker, freq uint) *ConstantAttacker {
	return &ConstantAttacker{
		genericAttacker: genericAttacker{
			trg: trg,
			atk: atk,
		},
		rate: vegeta.ConstantPacer{
			Freq: int(freq),
			Per:  time.Second,
		},
	}
}

// Attack implements Attacker.
func (a *ConstantAttacker) Attack(d time.Duration) vegeta.Metrics {
	var metrics vegeta.Metrics

	for res := range a.atk.Attack(a.trg, a.rate, d, "drill") {
		metrics.Add(res)
	}

	metrics.Close()

	return metrics
}

// String implements fmt.Stringer.
func (a *ConstantAttacker) String() string {
	return a.rate.String()
}

// RampAttacker runs an attack at increasing rates.
type RampAttacker struct {
	genericAttacker

	maxRate   vegeta.ConstantPacer
	intervals uint
}

var _ Attacker = (*RampAttacker)(nil)

// NewRampAttacker returns a new RampAttacker configured to attack in intervals
// at increasing rates.
func NewRampAttacker(trg vegeta.Targeter, atk *vegeta.Attacker, freq, intervals uint) (*RampAttacker, error) {
	if freq == 0 {
		return nil, fmt.Errorf("infinite rate (0) makes no sense for a ramp attack")
	}
	if freq/intervals == 0 {
		return nil, fmt.Errorf("a rate of %d events/sec divided in %d intervals would result in an interval "+
			"rate < 1 event/sec", freq, intervals)
	}

	return &RampAttacker{
		genericAttacker: genericAttacker{
			trg: trg,
			atk: atk,
		},

		maxRate: vegeta.ConstantPacer{
			Freq: int(freq),
			Per:  time.Second,
		},
		intervals: intervals,
	}, nil
}

// Attack implements Attacker.
func (a *RampAttacker) Attack(d time.Duration) vegeta.Metrics {
	var metrics vegeta.Metrics

	pcr := &RampPacer{
		IntervalRate: vegeta.ConstantPacer{
			// NOTE(antoineco): we expect the caller to understand
			// the potential reduction of the max rate due to the
			// integer division (e.g. 1234/5 * 5 = 1230)
			Freq: a.maxRate.Freq / int(a.intervals),
			Per:  time.Second,
		},
		IntervalDuration: time.Duration(int64(d) / int64(a.intervals)),
	}

	for res := range a.atk.Attack(a.trg, pcr, d, "drill") {
		metrics.Add(res)
	}

	metrics.Close()

	return metrics
}

// RampPacer paces an attack by starting at a constant interval rate for a
// given interval duration, then multiplies the initial rate by a whole number
// that gets incremented in each new interval.
type RampPacer struct {
	IntervalRate     vegeta.ConstantPacer
	IntervalDuration time.Duration
}

// Pace implements vegeta.Pacer.
func (p *RampPacer) Pace(elapsed time.Duration, hits uint64) (time.Duration, bool) {
	if p.IntervalRate.Freq == 0 {
		return 0, true // infinite rate makes no sense for a ramp attack
	}

	expectedHits := p.expectedHits(elapsed)
	if hits == 0 || hits < uint64(expectedHits) {
		// Running behind, send next hit immediately.
		return 0, false
	}

	rate := p.Rate(elapsed)
	interval := math.Round(float64(time.Second.Nanoseconds()) / rate)

	if n := uint64(interval); n != 0 && math.MaxInt64/n < hits {
		// We would overflow wait if we continued, so stop the attack.
		return 0, true
	}

	delta := float64(hits+1) - expectedHits
	wait := time.Duration(interval * delta)

	return wait, false
}

// Rate implements vegeta.Pacer.
// It returns the Pacer's expected instantaneous hit rate (i.e. requests per
// second) at the given elapsed duration of an attack.
func (p *RampPacer) Rate(elapsed time.Duration) float64 {
	currentInterval := int64(elapsed)/int64(p.IntervalDuration) + 1

	currentRate := vegeta.ConstantPacer{
		Freq: p.IntervalRate.Freq * int(currentInterval),
		Per:  p.IntervalRate.Per,
	}

	return currentRate.Rate(0)
}

// expectedHits returns the number of hits that are expected to have been sent
// at the given elapsed duration of an attack.
// It returns a float so we can tell exactly how much we've missed our target
// by when solving numerically in Pace.
func (p *RampPacer) expectedHits(elapsed time.Duration) float64 {
	if elapsed < 0 {
		return 0
	}

	currentInterval := int64(elapsed)/int64(p.IntervalDuration) + 1

	elapsedPreviousIntervals := time.Duration(int64(p.IntervalDuration) * (currentInterval - 1))
	elapsedCurrentInterval := elapsed - elapsedPreviousIntervals

	hits := (float64(elapsedCurrentInterval.Seconds()) * p.IntervalRate.Rate(0)) * float64(currentInterval)

	for i := int64(1); i < currentInterval; i++ {
		hits += (float64(p.IntervalDuration.Seconds()) * p.IntervalRate.Rate(0)) * float64(i)
	}

	return hits
}

// String implements fmt.Stringer.
func (a *RampAttacker) String() string {
	return fmt.Sprint("Ramp{", a.intervals, " intervals, ", a.maxRate.Freq/int(a.intervals), " hits/1s increments}")
}
