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
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/eventgrid/mgmt/eventgrid"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/triggermesh/test-infra/test/e2e/framework"
)

func CreateEventGridComponents(ctx context.Context, subscriptionID, name, region, rg string) error {
	egsubClient := eventgrid.NewEventSubscriptionsClient(subscriptionID)

	authorizer, err := auth.NewAuthorizerFromEnvironment()
	if err != nil {
		framework.FailfWithOffset(3, "unable to create authorizer: %s", err)
		return nil
	}

	egsubClient.Authorizer = authorizer

	return nil
}

func DeleteEventGridComponents(ctx context.Context, eventGrid string) error {
	return fmt.Errorf("unimplemented")
}
