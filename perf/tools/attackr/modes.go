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

import "strconv"

type mode uint8

const (
	modeConstantStr = "constant"
	modeRampStr     = "ramp"

	modeConstant mode = iota
	modeRamp
)

// String implements fmt.Stringer.
func (m mode) String() string {
	switch m {
	case modeConstant:
		return modeConstantStr
	case modeRamp:
		return modeRampStr
	default:
		return ""
	}
}

// errorUnsupportedMode indicates that the provided mode isn't supported.
type errorUnsupportedMode string

// Error implements the error interface.
func (e errorUnsupportedMode) Error() string {
	return "unsupported mode " + strconv.Quote(string(e))
}

// toMode returns the mode corresponding to the given input.
func toMode(in string) (*mode, error) {
	var m mode

	switch in {
	case modeConstantStr:
		m = modeConstant
	case modeRampStr:
		m = modeRamp
	default:
		return nil, errorUnsupportedMode(in)
	}

	return &m, nil
}
