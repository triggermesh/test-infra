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

package googlecloudpubsub

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"google.golang.org/api/option"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"

	duckv1 "knative.dev/pkg/apis/duck/v1"

	"cloud.google.com/go/pubsub"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/triggermesh/test-infra/test/e2e/framework"
	"github.com/triggermesh/test-infra/test/e2e/framework/apps"
	"github.com/triggermesh/test-infra/test/e2e/framework/bridges"
	"github.com/triggermesh/test-infra/test/e2e/framework/ducktypes"
	e2epubsub "github.com/triggermesh/test-infra/test/e2e/framework/gcloud/pubsub"
)

/* This test suite requires:

   - Google Cloud Credentials in json exported in the environment as GOOGLECLOUD_PUBSUB_KEY
   - The name of the Google Cloud project exported in the environment as GOOGLECLOUD_PROJECT
*/

var sourceAPIVersion = schema.GroupVersion{
	Group:   "sources.triggermesh.io",
	Version: "v1alpha1",
}

const (
	sourceKind     = "GoogleCloudPubSubSource"
	sourceResource = "googlecloudpubsubsources"
)

var _ = Describe("Google Cloud PubSub source", func() {
	f := framework.New("googlecloudpubsubsource")

	var ns string

	var srcClient dynamic.ResourceInterface

	var topic *pubsub.Topic
	var project string
	var saKey string

	var sink *duckv1.Destination

	BeforeEach(func() {
		ns = f.UniqueName

		gvr := sourceAPIVersion.WithResource(sourceResource)
		srcClient = f.DynamicClient.Resource(gvr).Namespace(ns)
	})

	Context("a source watches an non existing subscription", func() {
		var pubsubClient *pubsub.Client
		var err error

		BeforeEach(func() {
			saKey = e2epubsub.GetCreds()
			project = e2epubsub.GetProject()
			pubsubClient, err = pubsub.NewClient(context.Background(), project, option.WithCredentialsJSON([]byte(saKey)))
			Expect(err).ToNot(HaveOccurred())

			By("creating an event sink", func() {
				sink = bridges.CreateEventDisplaySink(f.KubeClient, ns)
			})

			By("creating a pubsub topic", func() {
				topic = e2epubsub.CreateTopic(pubsubClient, f)
			})

			By("creating a GoogleCloudPubSub object", func() {
				src, err := createSource(srcClient, ns, "test-", sink,
					withTopic(topic.String()),
					withCredentials(saKey),
				)
				Expect(err).ToNot(HaveOccurred())
				ducktypes.WaitUntilReady(f.DynamicClient, src)
			})
		})

		AfterEach(func() {
			By("deleting pubsub topic "+topic.String(), func() {
				e2epubsub.DeleteTopic(pubsubClient, topic)
			})
		})

		When("a message is sent to the topic", func() {
			var msgId string

			BeforeEach(func() {
				msgId = e2epubsub.SendMessage(pubsubClient, topic, f)
			})

			Specify("the source generates an event", func() {
				const receiveTimeout = 15 * time.Second
				const pollInterval = 500 * time.Millisecond

				var receivedEvents []cloudevents.Event

				readReceivedEvents := readReceivedEvents(f.KubeClient, ns, sink.Ref.Name, &receivedEvents)

				Eventually(readReceivedEvents, receiveTimeout, pollInterval).ShouldNot(BeEmpty())
				Expect(receivedEvents).To(HaveLen(1))

				e := receivedEvents[0]

				Expect(e.Type()).To(Equal("com.google.cloud.pubsub.message"))
				Expect(e.ID()).To(Equal(msgId))
				Expect(e.Source()).To(Equal(topic.String()))
			})
		})
	})

	When("a client creates a source object with invalid specs", func() {

		// Those tests do not require a real repository or sink
		BeforeEach(func() {
			saKey = "fake-creds"

			sink = &duckv1.Destination{
				Ref: &duckv1.KReference{
					APIVersion: "fake/v1",
					Kind:       "Fake",
					Name:       "fake",
				},
			}
		})

		// Here we use
		//   "Specify: the API server rejects ..., By: setting an invalid ..."
		// instead of
		//   "When: it sets an invalid ..., Specify: the API server rejects ..."
		// to avoid creating a namespace for each spec, due to their simplicity.
		Specify("the API server rejects the creation of that object", func() {

			By("setting an invalid topic", func() {
				invalidTopic := "projects/fake-project/topics//"

				_, err := createSource(srcClient, ns, "test-invalid-topic-", sink,
					withTopic(invalidTopic),
					withCredentials(saKey),
				)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("spec.topic: Invalid value: "))
			})

		})
	})
})

// createSource creates an GoogleCloudPubSub object initialized with the given options.
func createSource(srcClient dynamic.ResourceInterface, namespace, namePrefix string,
	sink *duckv1.Destination, opts ...sourceOption) (*unstructured.Unstructured, error) {

	src := &unstructured.Unstructured{}
	src.SetAPIVersion(sourceAPIVersion.String())
	src.SetKind(sourceKind)
	src.SetNamespace(namespace)
	src.SetGenerateName(namePrefix)

	if err := unstructured.SetNestedMap(src.Object, ducktypes.DestinationToMap(sink), "spec", "sink"); err != nil {
		framework.FailfWithOffset(2, "Failed to set spec.sink field: %s", err)
	}

	for _, opt := range opts {
		opt(src)
	}

	return srcClient.Create(context.Background(), src, metav1.CreateOptions{})
}

type sourceOption func(*unstructured.Unstructured)

func withTopic(topic string) sourceOption {
	return func(src *unstructured.Unstructured) {
		if err := unstructured.SetNestedField(src.Object, topic, "spec", "topic"); err != nil {
			framework.FailfWithOffset(3, "Failed to set spec.topic field: %s", err)
		}
	}
}

func withCredentials(creds string) sourceOption {
	c := map[string]interface{}{}
	c = map[string]interface{}{"value": creds}

	return func(src *unstructured.Unstructured) {
		if err := unstructured.SetNestedField(src.Object, c, "spec", "serviceAccountKey"); err != nil {
			framework.FailfWithOffset(3, "Failed to set spec.serviceAccountKey field: %s", err)
		}
	}
}

// readReceivedEvents returns a function that reads CloudEvents received by the
// event-display application and stores the result as the value of the given
// `receivedEvents` variable.
// The returned function signature satisfies the contract expected by
// gomega.Eventually: no argument and one or more return values.
func readReceivedEvents(c clientset.Interface, namespace, eventDisplayDeplName string,
	receivedEvents *[]cloudevents.Event) func() []cloudevents.Event {

	return func() []cloudevents.Event {
		ev := bridges.ReceivedEventDisplayEvents(
			apps.GetLogs(c, namespace, eventDisplayDeplName),
		)
		*receivedEvents = ev
		return ev
	}
}
