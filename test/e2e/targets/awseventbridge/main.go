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

package awseventbridge

import (
	"context"
	"encoding/json"
	"net/url"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/eventbridge"
	"github.com/aws/aws-sdk-go/service/eventbridge/eventbridgeiface"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/aws-sdk-go/service/sqs/sqsiface"
	"github.com/aws/aws-sdk-go/service/sts"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/triggermesh/test-infra/test/e2e/framework"
	"github.com/triggermesh/test-infra/test/e2e/framework/apps"
	e2eeventbridge "github.com/triggermesh/test-infra/test/e2e/framework/aws/eventbridge"
	"github.com/triggermesh/test-infra/test/e2e/framework/aws/iam"
	e2esqs "github.com/triggermesh/test-infra/test/e2e/framework/aws/sqs"
	e2ece "github.com/triggermesh/test-infra/test/e2e/framework/cloudevents"
	"github.com/triggermesh/test-infra/test/e2e/framework/ducktypes"
)

/* This test suite requires:

   - AWS credentials in whichever form (https://docs.aws.amazon.com/sdk-for-go/api/aws/session/#hdr-Sessions_options_from_Shared_Config)
   - The name of an AWS region exported in the environment as AWS_REGION
*/

var targetAPIVersion = schema.GroupVersion{
	Group:   "targets.triggermesh.io",
	Version: "v1alpha1",
}

const (
	targetKind     = "AWSEventBridgeTarget"
	targetResource = "awseventbridgetargets"
)

var _ = Describe("AWS EventBridge target", func() {
	f := framework.New("awseventbridgetarget")

	var ns string

	var trgtClient dynamic.ResourceInterface

	var awsAccountID string
	var awsRegion string

	BeforeEach(func() {
		ns = f.UniqueName

		gvr := targetAPIVersion.WithResource(targetResource)
		trgtClient = f.DynamicClient.Resource(gvr).Namespace(ns)
	})

	Context("a target generates a partner event source", func() {
		var trgtName string
		var trgtURL *url.URL

		var ebClient eventbridgeiface.EventBridgeAPI

		var eventSourceName string
		var eventBusName string

		BeforeEach(func() {
			sess := session.Must(session.NewSession())

			ident, err := sts.New(sess).GetCallerIdentity(nil)
			Expect(err).ToNot(HaveOccurred())

			awsAccountID = *ident.Account
			awsRegion = *sess.Config.Region

			ebClient = eventbridge.New(sess)

			By("creating an AWSEventBridgeTarget object", func() {
				trgt, err := createTarget(trgtClient, ns, "test-",
					withAccountID(awsAccountID),
					withRegion(awsRegion),
				)
				Expect(err).ToNot(HaveOccurred())

				trgt = ducktypes.WaitUntilReady(f.DynamicClient, trgt)

				trgtURL = ducktypes.Address(trgt)
				Expect(trgtURL).ToNot(BeNil())

				trgtName = trgt.GetName()
				eventSourceName = partnerEventSourceName(trgt)
			})

			By("associating the partner event source with an event bus", func() {
				eventBusName = e2eeventbridge.AssociatePartnerEventSource(ebClient, f, eventSourceName)

				evSrcInfo := e2eeventbridge.DescribeEventSource(ebClient, eventSourceName)
				Expect(*evSrcInfo.State).To(Equal(eventbridge.EventSourceStateActive))
			})
		})

		AfterEach(func() {

			By("deleting partner event bus "+eventBusName, func() {
				e2eeventbridge.DeleteEventBus(ebClient, eventBusName)
			})
		})

		When("an event is sent to the target", func() {
			var sentEvent *cloudevents.Event

			var ruleName string
			var ruleARN string

			var sqsClient sqsiface.SQSAPI
			var sqsQueueURL string
			var sqsQueueARN string

			BeforeEach(func() {

				By("creating a rule that matches on the partner event source", func() {
					pattern := e2eeventbridge.MatchSourcePattern(eventSourceName)
					ruleName, ruleARN = e2eeventbridge.CreateRule(ebClient, f, eventBusName, pattern)
				})

				By("creating a SQS queue which accepts messages from the rule", func() {
					sqsClient = sqs.New(session.Must(session.NewSession()))

					sqsQueueURL = e2esqs.CreateQueue(sqsClient, f)
					sqsQueueARN = e2esqs.QueueARN(sqsClient, sqsQueueURL)

					sqsPolicy := iam.NewPolicy(
						e2eeventbridge.NewEventBridgeToSQSPolicyStatement(ruleARN, sqsQueueARN),
					)
					e2esqs.SetQueuePolicy(sqsClient, sqsQueueURL, sqsPolicy)
				})

				By("setting the SQS queue as an event target in the rule", func() {
					e2eeventbridge.SetRuleTarget(ebClient, sqsQueueARN, ruleName, eventBusName)
				})

				By("sending an event", func() {
					sentEvent = e2ece.NewEvent(f)
					job := e2ece.RunEventSender(f.KubeClient, ns, trgtURL.String(), sentEvent)
					apps.WaitForCompletion(f.KubeClient, job)
				})
			})

			AfterEach(func() {

				By("deleting rule "+ruleName, func() {
					e2eeventbridge.DeleteRule(ebClient, ruleName, eventBusName)
				})

				By("deleting SQS queue "+sqsQueueURL, func() {
					e2esqs.DeleteQueue(sqsClient, sqsQueueURL)
				})
			})

			It("forwards the event to the partner event source", func() {
				var receivedMsg []byte

				By("polling the SQS queue", func() {
					const receiveTimeout = 15 * time.Second
					const pollInterval = 500 * time.Millisecond

					var receivedMsgs []*sqs.Message

					receiveMessages := receiveMessages(sqsClient, sqsQueueURL, &receivedMsgs)

					Eventually(receiveMessages, receiveTimeout, pollInterval).ShouldNot(BeEmpty(),
						"A message should have been received in the SQS queue")
					Expect(receivedMsgs).To(HaveLen(1),
						"Received %d messages instead of 1", len(receivedMsgs))

					receivedMsg = []byte(*receivedMsgs[0].Body)
				})

				By("inspecting the message payload", func() {
					msgData := make(map[string]interface{})
					err := json.Unmarshal(receivedMsg, &msgData)
					Expect(err).ToNot(HaveOccurred())

					Expect(msgData["source"]).To(Equal(eventSourceName),
						"The message was not emitted by the partner event source")

					eventData, err := json.Marshal(msgData["detail"])
					Expect(err).ToNot(HaveOccurred())

					gotEvent := &cloudevents.Event{}
					err = json.Unmarshal(eventData, gotEvent)
					Expect(err).ToNot(HaveOccurred())

					Expect(gotEvent.ID()).To(Equal(sentEvent.ID()))
					Expect(gotEvent.Type()).To(Equal(sentEvent.Type()))
					Expect(gotEvent.Source()).To(Equal(sentEvent.Source()))
					Expect(gotEvent.Data()).To(Equal(sentEvent.Data()))
					Expect(gotEvent.Extensions()[e2ece.E2ECeExtension]).
						To(Equal(sentEvent.Extensions()[e2ece.E2ECeExtension]))
				})
			})
		})

		When("the partner event bus gets deleted", func() {
			const stateCheckTimeout = 20 * time.Second
			const pollInterval = 500 * time.Millisecond

			BeforeEach(func() {

				By("ensuring the target has reported the current ACTIVE state", func() {
					eventSourceState := partnerEventSourceStateChecker(trgtClient, trgtName)
					Eventually(eventSourceState, stateCheckTimeout, pollInterval).
						Should(Equal(eventbridge.EventSourceStateActive))
				})

				By("deleting partner event bus "+eventBusName, func() {
					e2eeventbridge.DeleteEventBus(ebClient, eventBusName)
				})
			})

			Specify("the status of the target transitions to PENDING", func() {
				eventSourceState := partnerEventSourceStateChecker(trgtClient, trgtName)
				Eventually(eventSourceState, stateCheckTimeout, pollInterval).
					Should(Equal(eventbridge.EventSourceStatePending))
			})
		})
	})

	When("a client creates a target object with invalid specs", func() {

		BeforeEach(func() {
			awsAccountID = "123456789012"
			awsRegion = "us-east-2"
		})

		// Here we use
		//   "Specify: the API server rejects ..., By: setting an invalid ..."
		// instead of
		//   "When: it sets an invalid ..., Specify: the API server rejects ..."
		// to avoid creating a namespace for each spec, due to their simplicity.
		Specify("the API server rejects the creation of that object", func() {

			By("setting an invalid account ID", func() {
				invalidAccID := "0000"

				_, err := createTarget(trgtClient, ns, "test-invalid-accid-",
					withAccountID(invalidAccID),
					withRegion(awsRegion),
				)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("spec.accountID: Invalid value: "))
			})

			By("setting an invalid region", func() {
				invalidRegion := "not-a-region"

				_, err := createTarget(trgtClient, ns, "test-invalid-region-",
					withAccountID(awsAccountID),
					withRegion(invalidRegion),
				)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("spec.region: Invalid value: "))
			})
		})
	})
})

// createTarget creates an AWSEventBridgeTarget object initialized with the given options.
func createTarget(trgtClient dynamic.ResourceInterface, namespace, namePrefix string,
	opts ...targetOption) (*unstructured.Unstructured, error) {

	trgt := &unstructured.Unstructured{}
	trgt.SetAPIVersion(targetAPIVersion.String())
	trgt.SetKind(targetKind)
	trgt.SetNamespace(namespace)
	trgt.SetGenerateName(namePrefix)

	for _, opt := range opts {
		opt(trgt)
	}

	return trgtClient.Create(context.Background(), trgt, metav1.CreateOptions{})
}

type targetOption func(*unstructured.Unstructured)

func withAccountID(accID string) targetOption {
	return func(src *unstructured.Unstructured) {
		if err := unstructured.SetNestedField(src.Object, accID, "spec", "accountID"); err != nil {
			framework.FailfWithOffset(3, "Failed to set spec.accountID field: %s", err)
		}
	}
}

func withRegion(region string) targetOption {
	return func(src *unstructured.Unstructured) {
		if err := unstructured.SetNestedField(src.Object, region, "spec", "region"); err != nil {
			framework.FailfWithOffset(3, "Failed to set spec.region field: %s", err)
		}
	}
}

// partnerEventSourceName returns the name of the partner event source
// associated with the given target object.
func partnerEventSourceName(trgt *unstructured.Unstructured) string {
	name, found, err := unstructured.NestedString(trgt.Object, "status", "partnerEventSource", "name")
	if err != nil {
		framework.FailfWithOffset(2, "Error reading status.partnerEventSource.name field: %s", err)
	}
	if !found {
		framework.FailfWithOffset(2, "Then name of the partner event source was not found in the status")
	}

	return name
}

// partnerEventSourceStateChecker returns a function that gets a target from the
// cluster and returns the state of the partner event source associated with it.
// The returned function signature satisfies the contract expected by
// gomega.Eventually: no argument and one or more return values.
func partnerEventSourceStateChecker(c dynamic.ResourceInterface, trgtName string) func() string /*state*/ {
	return func() string {
		trgt, err := c.Get(context.Background(), trgtName, metav1.GetOptions{})
		if err != nil {
			framework.FailfWithOffset(2, "Failed to get target object: %s", err)
		}

		state, found, err := unstructured.NestedString(trgt.Object, "status", "partnerEventSource", "state")
		if err != nil {
			framework.FailfWithOffset(2, "Error reading status.partnerEventSource.state field: %s", err)
		}
		if !found {
			framework.FailfWithOffset(2, "Then state of the partner event source was not found in the status")
		}

		return state
	}
}

// receiveMessages returns a function that retrieves messages from the given
// SQS queue and stores the result as the value of the given `receivedMsgs`
// variable.
// The returned function signature satisfies the contract expected by
// gomega.Eventually: no argument and one or more return values.
func receiveMessages(sqsClient sqsiface.SQSAPI, queueURL string, receivedMsgs *[]*sqs.Message) func() []*sqs.Message {
	return func() []*sqs.Message {
		msgs := e2esqs.ReceiveMessages(sqsClient, queueURL)
		*receivedMsgs = msgs
		return msgs
	}
}
