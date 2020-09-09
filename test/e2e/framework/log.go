/*
Copyright (c) 2020 TriggerMesh Inc.

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

package framework

import (
	"fmt"

	"github.com/onsi/ginkgo"
)

// Logf logs the given message.
func Logf(format string, args ...interface{}) {
	fmt.Fprintf(ginkgo.GinkgoWriter, format+"\n", args...)
}

// Failf fails the test with the given message.
func Failf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)

	// offset level above caller
	// e.g. "It(...) -> f -> Fail(..., 2)" will be logged for "It"
	const offset = 2
	ginkgo.Fail(msg, offset)
}
