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

package azureeventhubs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	eventhubs "github.com/Azure/azure-event-hubs-go"
	"github.com/Azure/azure-sdk-for-go/services/eventhub/mgmt/2017-04-01/eventhub"
	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2017-05-10/resources"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/go-autorest/autorest/to"
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
	sourceKind     = "AzureEventHubSource"
	sourceResource = "azureeventhubsource"
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
 * Instantiate the AzureEventHubSource
 * Send an event to the AzureEventHubSource and look for a response
*/

var _ = FDescribe("Azure EventHubs", func() {
	ctx := context.Background()
	subscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID")
	region := "westus2" // Default to WestUS2 for now

	f := framework.New(sourceResource)

	var ns string
	var srcClient dynamic.ResourceInterface
	var sink *duckv1.Destination

	var rg resources.Group
	var hub *eventhubs.Hub

	BeforeEach(func() {
		ns = f.UniqueName
		gvr := sourceAPIVersion.WithResource(sourceResource + "s")
		srcClient = f.DynamicClient.Resource(gvr).Namespace(ns)

		rg = azure.CreateResourceGroup(ctx, subscriptionID, ns, region)
		hub = createEventHubComponents(ctx, subscriptionID, ns, region, rg)

	})

	Context("a source watches an EventHub", func() {
		var err error // stubbed

		When("an event flows", func() {
			It("should create an azure eventhub", func() {
				By("creating an event sink", func() {
					sink = bridges.CreateEventDisplaySink(f.KubeClient, ns)
				})

				var src *unstructured.Unstructured
				By("creating the azureeventhubsource", func() {
					src, err = createSource(srcClient, ns, "test-", sink,
						withServicePrincipal(),
						withSubscriptionID(subscriptionID),
						withEventHubID(createEventhubID(subscriptionID, ns)),
					)

					Expect(err).ToNot(HaveOccurred())

					ducktypes.WaitUntilReady(f.DynamicClient, src)
					time.Sleep(5 * time.Second)
				})

				By("sending an event", func() {
					ev := eventhubs.NewEvent([]byte("hello world"))
					err = hub.Send(ctx, ev, eventhubs.SendWithMessageID("12345"))
					Expect(err).ToNot(HaveOccurred())
				})

				By("verifying the event was sent", func() {
					const receiveTimeout = 15 * time.Second // it takes events a little longer to flow in from azure
					const pollInterval = 500 * time.Millisecond

					var receivedEvents []cloudevents.Event

					readReceivedEvents := readReceivedEvents(f.KubeClient, ns, sink.Ref.Name, &receivedEvents)

					Eventually(readReceivedEvents, receiveTimeout, pollInterval).ShouldNot(BeEmpty())
					Expect(receivedEvents).To(HaveLen(1))

					e := receivedEvents[0]

					Expect(e.Type()).To(Equal("com.microsoft.azure.eventhub.message"))
					Expect(e.Source()).To(Equal(createEventhubID(subscriptionID, ns)))

					data := make(map[string]interface{})
					err = json.Unmarshal(e.Data(), &data)
					testID := fmt.Sprintf("%v", data["ID"])
					Expect(data["ID"]).To(Equal(testID))
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

func withEventHubID(id string) sourceOption {
	return func(src *unstructured.Unstructured) {
		if err := unstructured.SetNestedField(src.Object, id, "spec", "eventHubID"); err != nil {
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
func createEventhubID(subscriptionID, testName string) string {
	return "/subscriptions/" + subscriptionID + "/resourceGroups/" + testName + "/providers/Microsoft.EventHub/namespaces/" + testName + "/eventHubs/" + testName
}

// createEventHubComponents will create an eventhubs namespace and eventhub using the given name
func createEventHubComponents(ctx context.Context, subscriptionID, name, region string, rg resources.Group) *eventhubs.Hub {
	nsClient := eventhub.NewNamespacesClient(subscriptionID)
	ehClient := eventhub.NewEventHubsClient(subscriptionID)

	authorizer, err := auth.NewAuthorizerFromEnvironment()
	if err != nil {
		framework.FailfWithOffset(3, "unable to create authorizer: %s", err)
		return nil
	}

	nsClient.Authorizer = authorizer
	ehClient.Authorizer = authorizer

	// create the eventhubs namespace
	nsFuture, err := nsClient.CreateOrUpdate(ctx, *rg.Name, name, eventhub.EHNamespace{Location: to.StringPtr(region)})
	if err != nil {
		framework.FailfWithOffset(3, "unable to create eventhubs namespace: %s", err)
		return nil
	}

	// Wait for the namespace to be created before creating the eventhub
	err = nsFuture.WaitForCompletionRef(ctx, nsClient.Client)
	if err != nil {
		framework.FailfWithOffset(3, "unable to complete eventhubs namespace creation: %s", err)
		return nil
	}

	ns, err := nsFuture.Result(nsClient)
	if err != nil {
		framework.FailfWithOffset(3, "eventhubs namespace creation failed: %s", err)
		return nil
	}

	_, err = ehClient.CreateOrUpdate(ctx, *rg.Name, *ns.Name, name, eventhub.Model{
		Properties: &eventhub.Properties{
			PartitionCount: to.Int64Ptr(2),
		},
	})

	if err != nil {
		framework.FailfWithOffset(3, "unable to create eventhub: %s", err)
		return nil
	}

	keys, err := nsClient.ListKeys(ctx, *rg.Name, *ns.Name, "RootManageSharedAccessKey")
	if err != nil {
		framework.FailfWithOffset(3, "unable to obtain the connection string: %s", err)
		return nil
	}

	// Take the namespace connection string, and add the specific eventhub
	connectionString := *keys.PrimaryConnectionString + ";EntityPath=" + name
	hub, err := eventhubs.NewHubFromConnectionString(connectionString)
	if err != nil {
		framework.FailfWithOffset(3, "unable to create eventhub client: %s", err)
		return nil
	}

	return hub
}
