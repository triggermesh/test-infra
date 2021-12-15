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

package azure

import (
	"context"

	eventhubs "github.com/Azure/azure-event-hubs-go"
	"github.com/Azure/azure-sdk-for-go/services/eventhub/mgmt/2017-04-01/eventhub"
	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2017-05-10/resources"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/triggermesh/test-infra/test/e2e/framework"
)

func CreateEventHubComponents(ctx context.Context, subscriptionID, name, region string, rg resources.Group) *eventhubs.Hub {
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
