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

// Package manifest contains helpers to consume objects from Kubernetes manifests.
package manifest

import (
	"io/ioutil"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/deprecated/scheme"

	"github.com/triggermesh/test-infra/test/e2e/framework"
)

// ObjectFromFile reads a JSON/YAML manifest and unmarshals its contents into
// an unstructured.Unstructured.
func ObjectFromFile(path string) *unstructured.Unstructured {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		framework.FailfWithOffset(2, "Error reading file: %s", err)
	}

	json, err := yaml.ToJSON(data)
	if err != nil {
		framework.FailfWithOffset(2, "Failed converting YAML to JSON: %s", err)
	}

	u := &unstructured.Unstructured{}

	if err := runtime.DecodeInto(scheme.Codecs.UniversalDecoder(), json, u); err != nil {
		framework.FailfWithOffset(2, "Failed decoding data into object: %s", err)
	}

	return u
}
