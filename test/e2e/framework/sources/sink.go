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

package sources

import (
	"bytes"
	"io"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	duckv1 "knative.dev/pkg/apis/duck/v1"

	"github.com/triggermesh/test-infra/test/e2e/framework"
	"github.com/triggermesh/test-infra/test/e2e/framework/deployment"
)

const (
	eventDisplayName           = "event-display"
	eventDisplayContainerImage = "gcr.io/knative-releases/knative.dev/eventing-contrib/cmd/event_display:latest"
)

// CreateEventDisplaySink creates an event-display event sink and returns it as
// a duckv1.Destination.
func CreateEventDisplaySink(cli kubernetes.Interface, namespace string) *duckv1.Destination {
	const internalPort uint16 = 8080
	const exposedPort uint16 = 80

	_, svc := deployment.CreateSimpleApplication(cli, namespace,
		eventDisplayName, eventDisplayContainerImage, internalPort, exposedPort)

	svcGVK := corev1.SchemeGroupVersion.WithKind("Service")

	return &duckv1.Destination{
		Ref: &duckv1.KReference{
			APIVersion: svcGVK.GroupVersion().String(),
			Kind:       svcGVK.Kind,
			Name:       svc.Name,
		},
	}
}

// ReceivedEventDisplayEvents returns all events received by the instance of
// event-display in the given namespace.
func ReceivedEventDisplayEvents(cli kubernetes.Interface, namespace string) []string {
	const delimiter = '☁'

	logStream := deployment.GetLogs(cli, namespace, eventDisplayName)
	defer func() {
		if err := logStream.Close(); err != nil {
			framework.FailfWithOffset(2, "Failed to close event-display's log stream: %s", err)
		}
	}()

	var buf bytes.Buffer

	// read everything at once instead of per chunk, because a buffered
	// read could end in the middle of a delimiter rune, causing an entire
	// event to be overlooked while parsing
	if _, err := buf.ReadFrom(logStream); err != nil {
		framework.FailfWithOffset(2, "Error reading event-display's log stream: %s", err)
	}

	eventBuilders := make([]strings.Builder, 0)
	currentEventIdx := -1

	for {
		r, _, err := buf.ReadRune()
		if err == io.EOF {
			break
		}
		if err != nil {
			framework.FailfWithOffset(2, "Error reading buffer: %s", err)
		}

		if r == delimiter {
			eventBuilders = append(eventBuilders, make([]strings.Builder, 1)...)
			currentEventIdx++
		}
		// everything until the first found delimiter will be ignored
		if currentEventIdx >= 0 {
			eventBuilders[currentEventIdx].WriteRune(r)
		}
	}

	if len(eventBuilders) == 0 {
		return nil
	}

	events := make([]string, len(eventBuilders))

	for i, eb := range eventBuilders {
		events[i] = eb.String()
	}
	return events
}
