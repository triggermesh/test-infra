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

package bridges

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/triggermesh/test-infra/test/e2e/framework"
)

// Components returns the Bridge's components as a list of
// map[string]interface{}.
func Components(brdg *unstructured.Unstructured) []map[string]interface{} /*components*/ {
	compInterfaces, found, err := unstructured.NestedSlice(brdg.Object, "spec", "components")
	if err != nil {
		framework.FailfWithOffset(3, "Error reading Bridge components: %s", err)
	}
	if !found {
		framework.FailfWithOffset(3, "No component was found in the Bridge")
	}

	comps := make([]map[string]interface{}, len(compInterfaces))

	for i, comp := range compInterfaces {
		comps[i] = comp.(map[string]interface{})
	}

	return comps
}

// SetComponents sets a list of map[string]interface{} as the Bridge's
// components.
func SetComponents(brdg *unstructured.Unstructured, comps []map[string]interface{}) {
	compInterfaces := make([]interface{}, len(comps))

	for i, comp := range comps {
		compInterfaces[i] = comp
	}

	if err := unstructured.SetNestedSlice(brdg.Object, compInterfaces, "spec", "components"); err != nil {
		framework.FailfWithOffset(3, "Error setting Bridge components: %s", err)
	}
}

// SeekComponentByKind returns the index of the component identified by the
// given kind in a list of Bridge components.
func SeekComponentByKind(components []map[string]interface{}, kind string) int /*index*/ {
	idx := -1

	for i, c := range components {
		k, found, err := unstructured.NestedString(c, "object", "kind")
		if err != nil {
			framework.FailfWithOffset(3, "Error reading Bridge component's kind: %s", err)
		}
		if !found {
			framework.FailfWithOffset(3, "Found component without kind at index %d", i)
		}

		if k == "GitHubSource" {
			idx = i
			break
		}
	}

	if idx < 0 {
		framework.FailfWithOffset(3, "GitHubSource component was not found in the Bridge")
	}
	return idx
}
