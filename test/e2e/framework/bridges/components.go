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

package bridges

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/triggermesh/test-infra/test/e2e/framework"
)

type BridgeOption func(*unstructured.Unstructured)

// CreateBridge creates a Bridge object initialized with the given options.
func CreateBridge(brdgCli dynamic.ResourceInterface, bridge *unstructured.Unstructured,
	namespace, namePrefix string, opts ...BridgeOption) (*unstructured.Unstructured, error) {

	bridge.SetNamespace(namespace)
	bridge.SetGenerateName(namePrefix)

	for _, opt := range opts {
		opt(bridge)
	}

	return brdgCli.Create(context.Background(), bridge, metav1.CreateOptions{})
}

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
	idx := seekComponentByKindAndName(components, kind, "")

	if idx < 0 {
		framework.FailfWithOffset(3, "No component found in the Bridge for kind %s", kind)
	}

	return idx
}

// SeekComponentByKindAndName returns the index of the component identified by
// the given kind and name in a list of Bridge components.
func SeekComponentByKindAndName(components []map[string]interface{}, kind, name string) int /*index*/ {
	idx := seekComponentByKindAndName(components, kind, name)

	if idx < 0 {
		framework.FailfWithOffset(3, "No component found in the Bridge for kind %s and name %q", kind, name)
	}

	return idx
}

func seekComponentByKindAndName(components []map[string]interface{}, kind, name string) int /*index*/ {
	idx := -1

	for i, c := range components {
		k, found, err := unstructured.NestedString(c, "object", "kind")
		if err != nil {
			framework.FailfWithOffset(4, "Error reading Bridge component's kind: %s", err)
		}
		if !found {
			framework.FailfWithOffset(4, "Found component without kind at index %d", i)
		}

		if k == kind {
			if name != "" {
				n, found, err := unstructured.NestedString(c, "object", "metadata", "name")
				if err != nil {
					framework.FailfWithOffset(4, "Error reading Bridge component's name: %s", err)
				}
				if !found {
					framework.FailfWithOffset(4, "Found component without name at index %d", i)
				}

				if n != name {
					continue
				}
			}

			idx = i
			break
		}
	}

	return idx
}
