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

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2017-05-10/resources"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/triggermesh/test-infra/test/e2e/framework"
)

// Package azure contains helpers for interacting with Azure and standing up prerequisite services

const E2EInstanceTagKey = "e2e_instance"

// CreateResourceGroup will create the resource group containing all of the eventhub components.
func CreateResourceGroup(ctx context.Context, subscriptionID, name, region string) resources.Group {
	rgClient := resources.NewGroupsClient(subscriptionID)
	authorizer, err := auth.NewAuthorizerFromEnvironment()
	if err != nil {
		framework.FailfWithOffset(3, "unable to create authorizer: %s", err)
		return resources.Group{}
	}

	rgClient.Authorizer = authorizer

	rg, err := rgClient.CreateOrUpdate(ctx, name, resources.Group{
		Location: to.StringPtr(region),
		Tags:     map[string]*string{E2EInstanceTagKey: to.StringPtr(name)},
	})

	if err != nil {
		framework.FailfWithOffset(3, "unable to create resource group: %s", err)
		return resources.Group{}
	}

	return rg
}

// DeleteResourceGroup will delete everything under it allowing for easy cleanup
func DeleteResourceGroup(ctx context.Context, subscriptionID, name string) resources.GroupsDeleteFuture {
	authorizer, err := auth.NewAuthorizerFromEnvironment()
	if err != nil {
		framework.FailfWithOffset(3, "unable to delete resource group: %s", err)
		return resources.GroupsDeleteFuture{}
	}

	rgClient := resources.NewGroupsClient(subscriptionID)
	rgClient.Authorizer = authorizer

	rgf, err := rgClient.Delete(ctx, name)
	if err != nil {
		framework.FailfWithOffset(3, "resource group deletion failed: %s", err)
	}

	return rgf
}

// WaitForFutureDeletion will wait on the resource to be deleted before continuing
func WaitForFutureDeletion(ctx context.Context, subscriptionID string, future resources.GroupsDeleteFuture) {
	authorizer, _ := auth.NewAuthorizerFromEnvironment()
	rgClient := resources.NewGroupsClient(subscriptionID)
	rgClient.Authorizer = authorizer

	err := future.WaitForCompletionRef(ctx, rgClient.Client)
	if err != nil {
		framework.FailfWithOffset(3, "resource group deletion failed: %s", err)
	}
}
