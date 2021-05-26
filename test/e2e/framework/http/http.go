/*
Copyright (c) 2021 TriggerMesh Inc.

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

// Package http contains helpers related to HTTP protocol
package http

import (
	"bytes"
	"net/http"

	"github.com/triggermesh/test-infra/test/e2e/framework"
)

// PostJSONRequest send an arbitraty JSON payload to an endpoint.
func PostJSONRequest(url string, payload []byte) {
	res, err := http.Post(url, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		framework.FailfWithOffset(2, "Error Posting to %s: %s", url, err)
	}

	if res.StatusCode >= 400 {
		framework.FailfWithOffset(2, "Posting to %s returned error code %d", url, res.StatusCode)
	}
}
