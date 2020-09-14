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
	"bufio"
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"strings"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/event/datacodec"

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
func ReceivedEventDisplayEvents(cli kubernetes.Interface, namespace string) []cloudevents.Event {
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

	events := make([]cloudevents.Event, len(eventBuilders))

	for i, eb := range eventBuilders {
		events[i] = parseCloudEvent(eb.String())
	}
	return events
}

// parseCloudEvent parses the content of a stringified CloudEvent into a
// structured CloudEvent.
//
// Example of output from Event.String():
//
// ☁  cloudevents.Event
// Validation: valid
// Context Attributes,
//   specversion: 1.0
//   type: io.triggermesh.some.event
//   source: some/source
//   subject: some-subject
//   id: edecf007-f651-4e10-959e-e2f0a5b8ccd0
//   time: 2020-09-14T13:59:40.693213706Z
//   datacontenttype: application/json
// Data,
//   {
//     ...
//   }
func parseCloudEvent(ce string) cloudevents.Event {
	e := cloudevents.NewEvent()

	contentType := cloudevents.ApplicationJSON

	r := bufio.NewReader(strings.NewReader(ce))

	for {
		line, err := r.ReadString('\n')
		line = strings.TrimSpace(line)

		// try finding context attributes and data
		subs := strings.SplitN(line, ":", 2)

		switch len(subs) {
		case 1:
			if subs[0] != "Data," {
				break
			}

			// read everything that's left to read and set
			// it as the event's data
			b, err := ioutil.ReadAll(r)
			if err != nil {
				framework.Logf("Error reading event's data: %s", err)
				break
			}

			decodedData := make(map[string]interface{})

			if err := datacodec.Decode(context.Background(), e.DataMediaType(), b, &decodedData); err != nil {
				framework.Logf("Error decoding event's data: %s", err)
				e.SetData(contentType, b)
			} else {
				e.SetData(contentType, decodedData)
			}

		case 2:
			switch k, v := subs[0], strings.TrimSpace(subs[1]); k {
			case "datacontenttype":
				contentType = v
				e.SetDataContentType(v)
			case "type":
				e.SetType(v)
			case "source":
				e.SetSource(v)
			case "subject":
				e.SetSubject(v)
			case "id":
				e.SetID(v)
			case "time":
				t, err := time.Parse(time.RFC3339Nano, v)
				if err != nil {
					framework.Logf("Error parsing event's time: %s", err)
					break
				}
				e.SetTime(t)
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			framework.FailfWithOffset(3, "Error reading line from Reader: %s", err)
		}
	}

	return e
}
