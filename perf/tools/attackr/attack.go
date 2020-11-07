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
	"time"

	vegeta "github.com/tsenart/vegeta/v12/lib"
)

// Attacker is the interface implemented by custom vegeta Attackers.
type Attacker interface {
	// Attack performs an attack on a target.
	Attack(time.Duration) vegeta.Metrics
	// The embedded Stringer interface is implemented to allow returning a
	// human-readable description of the attack.
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
	pcr vegeta.Pacer
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
		pcr: vegeta.ConstantPacer{
			Freq: int(freq),
			Per:  time.Second,
		},
	}
}

// Attack implements Attacker.
func (a *ConstantAttacker) Attack(d time.Duration) vegeta.Metrics {
	var metrics vegeta.Metrics

	for res := range a.atk.Attack(a.trg, a.pcr, d, "drill") {
		metrics.Add(res)
	}

	metrics.Close()

	return metrics
}

// String implements fmt.Stringer.
func (a *ConstantAttacker) String() string {
	return a.pcr.(fmt.Stringer).String()
}
