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

package azureactivitylog

import (
	"context"
	"os"
	"time"

	eventhubs "github.com/Azure/azure-event-hubs-go"
	"github.com/Azure/azure-sdk-for-go/services/eventhub/mgmt/2017-04-01/eventhub"
	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2017-05-10/resources"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/triggermesh/test-infra/test/e2e/framework/azure"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/triggermesh/test-infra/test/e2e/framework"
	"github.com/triggermesh/test-infra/test/e2e/framework/apps"
	"github.com/triggermesh/test-infra/test/e2e/framework/bridges"
	"github.com/triggermesh/test-infra/test/e2e/framework/ducktypes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

/*
  This test requires:
  - Azure Service Principal Credentials with the Azure Event Hubs Data Owner role assigned at the subscription level

  The following environment variables _MUST_ be set:
  - AZURE_SUBSCRIPTION_ID - Common subscription for the test to run against
  - AZURE_TENANT_ID - Azure tenant to create the resources against
  - AZURE_CLIENT_ID - The Azure ServicePrincipal Client ID
  - AZURE_CLIENT_SECRET - The Azure ServicePrincipal Client Secret

  These will be done by the e2e test:
  - Create an Azure Resource Group, EventHubs Namespace, and EventHub
  - Send an event from the Azure EventHub into the TriggerMesh source

*/

var sourceAPIVersion = schema.GroupVersion{
	Group:   "sources.triggermesh.io",
	Version: "v1alpha1",
}

const (
	sourceKind     = "AzureActivityLogsSource"
	sourceResource = "azureactivitylogssource"
)

type AzureEventHubClient struct {
	NamespaceClient eventhub.NamespacesClient
	GroupClient     resources.GroupsClient
	HubClient       eventhub.EventHubsClient
	Hub             *eventhubs.Hub
}

/*
 Basic flow will resemble:
 * Create a resource group to contain our eventhub
 * Ensure our service principal can read/write from the eventhub
 * Instantiate the AzureActivityLogsSource
 * Create a resource group and watch the event flow in
*/

var _ = Describe("Azure Activity Logs", func() {
	ctx := context.Background()
	subscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID")
	region := "westus2" // Default to WestUS2 for now

	f := framework.New(sourceResource)

	var ns string
	var srcClient dynamic.ResourceInterface
	var sink *duckv1.Destination

	var rg resources.Group

	BeforeEach(func() {
		ns = f.UniqueName
		gvr := sourceAPIVersion.WithResource(sourceResource + "s")
		srcClient = f.DynamicClient.Resource(gvr).Namespace(ns)

		rg = azure.CreateResourceGroup(ctx, subscriptionID, ns, region)
		_ = azure.CreateEventHubComponents(ctx, subscriptionID, ns, region, rg)

	})

	Context("a source watches an EventHub publishing Activity Log data", func() {
		var err error // stubbed
		var testRG resources.Group

		When("an event flows", func() {
			It("should create an azure eventhub", func() {
				By("creating an event sink", func() {
					sink = bridges.CreateEventDisplaySink(f.KubeClient, ns)
				})

				By("creating a sample resource group to produce activity", func() {
					testRG = azure.CreateResourceGroup(ctx, subscriptionID, *rg.Name+"-testrg", region)
				})

				var src *unstructured.Unstructured
				By("creating the azureactivitylog source", func() {
					src, err = createSource(srcClient, ns, "test-", sink,
						withServicePrincipal(),
						withSubscriptionID(subscriptionID),
						withActivityCategories([]string{"Administrative", "Policy", "Security"}),
						withEventHubNS(createEventHubNS(subscriptionID, ns)),
						withEventHubID(ns),
					)

					Expect(err).ToNot(HaveOccurred())

					ducktypes.WaitUntilReady(f.DynamicClient, src)
				})

				By("verifying the event was sent by deleting a resource", func() {
					deleteFuture := azure.DeleteResourceGroup(ctx, subscriptionID, *testRG.Name)
					azure.WaitForFutureDeletion(ctx, subscriptionID, deleteFuture)

					const receiveTimeout = 300 * time.Second // It can take up to 5 minutes for an event to appear
					const pollInterval = 500 * time.Millisecond

					var receivedEvents []cloudevents.Event

					readReceivedEvents := readReceivedEvents(f.KubeClient, ns, sink.Ref.Name, &receivedEvents)

					Eventually(readReceivedEvents, receiveTimeout, pollInterval).ShouldNot(BeEmpty())
					Expect(receivedEvents).ToNot(BeEmpty()) // In some cases will receive either 1 or 2 events
				})
			})
		})
	})

	AfterEach(func() {
		_ = azure.DeleteResourceGroup(ctx, subscriptionID, *rg.Name)
	})
})

type sourceOption func(*unstructured.Unstructured)

// createSource creates an AzureEventHubSource object initialized with the test parameters
func createSource(srcClient dynamic.ResourceInterface, namespace, namePrefix string,
	sink *duckv1.Destination, opts ...sourceOption) (*unstructured.Unstructured, error) {
	src := &unstructured.Unstructured{}
	src.SetAPIVersion(sourceAPIVersion.String())
	src.SetKind(sourceKind)
	src.SetNamespace(namespace)
	src.SetGenerateName(namePrefix)

	// Set spec parameters

	if err := unstructured.SetNestedMap(src.Object, ducktypes.DestinationToMap(sink), "spec", "sink"); err != nil {
		framework.FailfWithOffset(2, "Failed to set spec.sink field: %s", err)
	}

	for _, opt := range opts {
		opt(src)
	}

	return srcClient.Create(context.Background(), src, metav1.CreateOptions{})
}

// Define the creation parameters to pass along

func withEventHubNS(ns string) sourceOption {
	return func(src *unstructured.Unstructured) {
		if err := unstructured.SetNestedField(src.Object, ns, "spec", "destination", "eventHubs", "namespaceID"); err != nil {
			framework.FailfWithOffset(3, "failed to set spec.destination.eventHubs.namespaceID: %s", err)
		}
	}
}

func withEventHubID(id string) sourceOption {
	return func(src *unstructured.Unstructured) {
		if err := unstructured.SetNestedField(src.Object, id, "spec", "destination", "eventHubs", "hubName"); err != nil {
			framework.FailfWithOffset(3, "failed to set spec.destination.eventHubs.hubName: %s", err)
		}
	}
}

func withActivityCategories(categories []string) sourceOption {
	// The make slice and for loop is to ensure the string array gets converted to an interface array
	iarray := make([]interface{}, len(categories))
	for i := range categories {
		iarray[i] = categories[i]
	}

	return func(src *unstructured.Unstructured) {
		if err := unstructured.SetNestedSlice(src.Object, iarray, "spec", "categories"); err != nil {
			framework.FailfWithOffset(3, "failed to set spec.eventHubID: %s", err)
		}
	}
}

func withSubscriptionID(id string) sourceOption {
	return func(src *unstructured.Unstructured) {
		if err := unstructured.SetNestedField(src.Object, id, "spec", "subscriptionID"); err != nil {
			framework.FailfWithOffset(3, "failed to set spec.subscriptionID: %s", err)
		}
	}
}

// withServicePrincipal will create the secret and service principal based on the azure environment variables
func withServicePrincipal() sourceOption {
	credsMap := map[string]interface{}{
		"tenantID":     map[string]interface{}{"value": os.Getenv("AZURE_TENANT_ID")},
		"clientID":     map[string]interface{}{"value": os.Getenv("AZURE_CLIENT_ID")},
		"clientSecret": map[string]interface{}{"value": os.Getenv("AZURE_CLIENT_SECRET")},
	}

	return func(src *unstructured.Unstructured) {
		if err := unstructured.SetNestedMap(src.Object, credsMap, "spec", "auth", "servicePrincipal"); err != nil {
			framework.FailfWithOffset(3, "Failed to set spec.auth.servicePrincipal field: %s", err)
		}
	}
}

// readReceivedEvents returns a function that reads CloudEvents received by the
// event-display application and stores the result as the value of the given
// `receivedEvents` variable.
// The returned function signature satisfies the contract expected by
// gomega.Eventually: no argument and one or more return values.
func readReceivedEvents(c clientset.Interface, namespace, eventDisplayName string,
	receivedEvents *[]cloudevents.Event) func() []cloudevents.Event {

	return func() []cloudevents.Event {
		ev := bridges.ReceivedEventDisplayEvents(
			apps.GetLogs(c, namespace, eventDisplayName),
		)
		*receivedEvents = ev
		return ev
	}
}

// createEventhubID will create the EventHub path used by the k8s azureeventhubssource
func createEventHubNS(subscriptionID, testName string) string {
	return "/subscriptions/" + subscriptionID + "/resourceGroups/" + testName + "/providers/Microsoft.EventHub/namespaces/" + testName
}
