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

package eventbridge

import (
	"github.com/google/uuid"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/eventbridge"
	"github.com/aws/aws-sdk-go/service/eventbridge/eventbridgeiface"

	"github.com/triggermesh/test-infra/test/e2e/framework"
	e2eaws "github.com/triggermesh/test-infra/test/e2e/framework/aws"
	"github.com/triggermesh/test-infra/test/e2e/framework/aws/iam"
)

// CreateRule creates a rule named after the given framework.Framework.
func CreateRule(ebClient eventbridgeiface.EventBridgeAPI, f *framework.Framework,
	eventBusName, pattern string) (string /*name*/, string /*arn*/) {

	rule := &eventbridge.PutRuleInput{
		Name:         aws.String("e2e-" + f.UniqueName),
		EventBusName: &eventBusName,
		EventPattern: &pattern,
		Tags:         tagsAsEventBridgeTags(e2eaws.TagsFor(f)),
	}

	resp, err := ebClient.PutRule(rule)
	if err != nil {
		framework.FailfWithOffset(2, "Failed to create rule %q: %s", *rule.Name, err)
	}

	return *rule.Name, *resp.RuleArn
}

// MatchSourcePattern returns a rule pattern that matches on the "source"
// attribute of an event.
//
// Sample event:
// {
//     "version": "0",
//     "id": "291b74d6-af33-2dcd-0ed6-b358bc2909ec",
//     "detail-type": "e2e.test",
//     "source": "aws.partner/triggermesh.com/123456789012/test-1234/sample",
//     "account": "123456789012",
//     "time": "2020-09-17T22:38:19Z",
//     "region": "eu-central-1",
//     "resources": [],
//     "detail": {
//         "data": "Hello, World",
//         "datacontenttype": "text/plain",
//         "id": "0000",
//         "iotriggermeshe2e": "test-1234",
//         "source": "e2e.triggermesh",
//         "specversion": "1.0",
//         "type": "e2e.test"
//     }
// }
//
func MatchSourcePattern(src string) string /*pattern*/ {
	return `{ "source": ["` + src + `"] }`
}

// DeleteRule deletes a rule and all its targets.
func DeleteRule(ebClient eventbridgeiface.EventBridgeAPI, name, eventBusName string) {
	removeAllTargets(ebClient, name, eventBusName)

	rule := &eventbridge.DeleteRuleInput{
		Name:         &name,
		EventBusName: &eventBusName,
	}

	if _, err := ebClient.DeleteRule(rule); err != nil {
		framework.FailfWithOffset(2, "Failed to delete rule %q: %s", *rule.Name, err)
	}
}

// removeAllTargets removes all targets from the given rule.
func removeAllTargets(ebClient eventbridgeiface.EventBridgeAPI, ruleName, eventBusName string) {
	rule := &eventbridge.ListTargetsByRuleInput{
		Rule:         &ruleName,
		EventBusName: &eventBusName,
	}

	trgts, err := ebClient.ListTargetsByRule(rule)
	if err != nil {
		framework.FailfWithOffset(3, "Failed to list targets for rule %q: %s", *rule.Rule, err)
	}

	trgtIDs := make([]*string, len(trgts.Targets))
	for i, trgt := range trgts.Targets {
		trgtIDs[i] = trgt.Id
	}

	trgtsToRemove := &eventbridge.RemoveTargetsInput{
		Rule:         &ruleName,
		EventBusName: &eventBusName,
		Ids:          trgtIDs,
	}

	if _, err := ebClient.RemoveTargets(trgtsToRemove); err != nil {
		framework.FailfWithOffset(3, "Failed to remove targets from rule %q: %s", *trgtsToRemove.Rule, err)
	}
}

// SetRuleTarget sets a target on a rule.
func SetRuleTarget(ebClient eventbridgeiface.EventBridgeAPI, targetARN, ruleName, eventBusName string) {
	trgt := &eventbridge.PutTargetsInput{
		Rule:         &ruleName,
		EventBusName: &eventBusName,
		Targets: []*eventbridge.Target{{
			Arn: &targetARN,
			Id:  aws.String(uuid.New().String()),
		}},
	}

	if _, err := ebClient.PutTargets(trgt); err != nil {
		framework.FailfWithOffset(2, "Failed to set target on rule %q: %s", *trgt.Rule, err)
	}
}

// NewEventBridgeToSQSPolicyStatement returns an IAM Policy Statement that
// allows messages matching the given rule to be sent to the given SQS queue.
func NewEventBridgeToSQSPolicyStatement(ruleARN, queueARN string) iam.PolicyStatement {
	return iam.NewPolicyStatement(iam.EffectAllow,
		iam.PrincipalService("events.amazonaws.com"),
		iam.ConditionArnEquals("aws:SourceArn", ruleARN),
		iam.Action("sqs:SendMessage"),
		iam.Resource(queueARN),
	)
}
