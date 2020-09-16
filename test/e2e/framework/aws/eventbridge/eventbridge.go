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

// Package eventbridge contains helpers for AWS EventBridge.
package eventbridge

import (
	"github.com/aws/aws-sdk-go/service/eventbridge"
	"github.com/aws/aws-sdk-go/service/eventbridge/eventbridgeiface"

	"github.com/triggermesh/test-infra/test/e2e/framework"
	e2eaws "github.com/triggermesh/test-infra/test/e2e/framework/aws"
)

// AssociatePartnerEventSource associates a partner event source with a partner
// event bus. The partner event source automatically becomes ACTIVE after this
// operation.
func AssociatePartnerEventSource(ebClient eventbridgeiface.EventBridgeAPI, f *framework.Framework,
	eventSourceName string) string /*event bus name*/ {

	eventBus := &eventbridge.CreateEventBusInput{
		// a partner event bus *must* have the same name as
		// the partner event source it is matched to
		Name:            &eventSourceName,
		EventSourceName: &eventSourceName,
		Tags:            tagsAsEventBridgeTags(e2eaws.TagsFor(f)),
	}

	if _, err := ebClient.CreateEventBus(eventBus); err != nil {
		framework.FailfWithOffset(2, "Failed to create event bus %q: %s", eventSourceName, err)
	}

	return *eventBus.Name
}

// DeleteEventBus deletes an event bus.
func DeleteEventBus(ebClient eventbridgeiface.EventBridgeAPI, eventBusName string) {
	eventBus := &eventbridge.DeleteEventBusInput{
		Name: &eventBusName,
	}

	if _, err := ebClient.DeleteEventBus(eventBus); err != nil {
		framework.FailfWithOffset(2, "Failed to delete event bus %q: %s", eventBusName, err)
	}
}

// DescribeEventSource returns details about the given event source.
func DescribeEventSource(ebClient eventbridgeiface.EventBridgeAPI, eventSourceName string) *eventbridge.DescribeEventSourceOutput {
	eventSrc := &eventbridge.DescribeEventSourceInput{
		Name: &eventSourceName,
	}

	eventSrcInfo, err := ebClient.DescribeEventSource(eventSrc)
	if err != nil {
		framework.FailfWithOffset(2, "Failed to describe event source %q: %s", eventSourceName, err)
	}

	return eventSrcInfo
}
