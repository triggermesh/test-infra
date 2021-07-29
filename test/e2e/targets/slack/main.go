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

package slack

import (
	"context"
	"log"
	"net/url"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/slack-go/slack"

	"github.com/triggermesh/test-infra/test/e2e/framework"
	"github.com/triggermesh/test-infra/test/e2e/framework/apps"
	e2ece "github.com/triggermesh/test-infra/test/e2e/framework/cloudevents"
	"github.com/triggermesh/test-infra/test/e2e/framework/ducktypes"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
)

/**
 * This test suite will require SLACK credentials passed as SLACK_E2E_ACCESS_TOKEN to submit a post.
 * The access token must have at least `chat:write` scope to submit the test message, and the `bot` scope to use Slack's
 * RTM library (see: https://api.slack.com/rtm)
 *
 * An alternate environment variable SLACK_E2E_TEST_CHANNEL is used to specify the target channel to publish messages and
 * monitor. By default, this is #e2e-slack-test. For other channels, the API key assigned to the bot must be invited to
 * the channel to monitor.
 */

var targetAPIVersion = schema.GroupVersion{
	Group:   "targets.triggermesh.io",
	Version: "v1alpha1",
}

const (
	targetKind     = "SlackTarget"
	targetResource = "slacktargets"
)

var _ = Describe("Slack target", func() {
	f := framework.New("slacktarget")

	var ns string
	var tgtClient dynamic.ResourceInterface
	var tgtURL *url.URL

	var err error

	var rtm *slack.RTM
	var receivedEvent chan slack.RTMEvent

	// Kill the websocket connection when finished. This will run regardless of failure state
	AfterSuite(func() {
		// Shut down the Slack service
		if rtm != nil {
			err = rtm.Disconnect()
			Expect(err).ToNot(HaveOccurred())
		}
	})

	BeforeEach(func() {
		ns = f.UniqueName

		gvr := targetAPIVersion.WithResource(targetResource)
		tgtClient = f.DynamicClient.Resource(gvr).Namespace(ns)

		var slackSecret *corev1.Secret

		By("creating a slack secret", func() {
			slackSecret, err = createSecret(f.KubeClient, ns, "slack-secret", os.Getenv("SLACK_E2E_ACCESS_TOKEN"))
			Expect(err).ToNot(HaveOccurred())
		})

		By("creating a SlackTarget object", func() {
			tgt, err := createTarget(tgtClient, ns, "test-", withSecret(slackSecret.Name))
			Expect(err).ToNot(HaveOccurred())

			tgt = ducktypes.WaitUntilReady(f.DynamicClient, tgt)

			tgtURL = ducktypes.Address(tgt)
			Expect(tgtURL).ToNot(BeNil())
		})
	})

	When("an event is sent to the target", func() {
		By("creating a slack webservice to monitor the results", func() {
			receivedEvent = make(chan slack.RTMEvent)
			api := slack.New(
				os.Getenv("SLACK_E2E_ACCESS_TOKEN"),
				slack.OptionDebug(false),
				slack.OptionLog(log.New(os.Stdout, "slack-target-test: ", log.Lshortfile|log.LstdFlags)))

			rtm = api.NewRTM()
			go rtm.ManageConnection()
			go captureEvents(rtm, receivedEvent)
		})

		It("posts a message", func() {
			sampleMsg := "this is a test message from: " + f.UniqueName
			targetChannel := os.Getenv("SLACK_E2E_TEST_CHANNEL")

			if targetChannel == "" {
				targetChannel = "e2e-slack-test"
			}

			msg := map[string]string{
				"channel": targetChannel,
				"text":    sampleMsg,
			}

			newEvent := e2ece.NewEvent(f)
			newEvent.SetType("com.slack.webapi.chat.postMessage")
			err := newEvent.SetData("application/json", msg)
			Expect(err).ToNot(HaveOccurred())

			job := e2ece.RunEventSender(f.KubeClient, ns, tgtURL.String(), newEvent)
			apps.WaitForCompletion(f.KubeClient, job)

			// Set up the timeout thread. If it takes longer than 5s to get a reply, then something is wrong
			ctx := context.Background()
			ctx, cancel := context.WithTimeout(ctx, time.Second*5)
			defer cancel()

			go func() {
				<-ctx.Done()
				close(receivedEvent)
			}()

			processed := false
			for slackEvent := range receivedEvent {
				switch se := slackEvent.Data.(type) {
				case *slack.MessageEvent:
					if se.Msg.Channel == targetChannel {
						Expect(se.Msg.Text).To(Equal(sampleMsg))
						processed = true
					}
				case *slack.RTMError:
					framework.Failf("received an error from slack: " + se.Error())
					processed = true
				case *slack.InvalidAuthEvent:
					framework.Failf("received an auth error from slack")
					processed = true
				}

				if processed {
					return
				}
			}

			if !processed {
				framework.Failf("timeout reached")
			}
		})
	})
})

// createSecret creates a slack secret to contain the API token
func createSecret(c clientset.Interface, namespace, namePrefix, token string) (*corev1.Secret, error) {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    namespace,
			GenerateName: namePrefix,
		},
		StringData: map[string]string{
			"token": token,
		},
	}
	return c.CoreV1().Secrets(namespace).Create(context.Background(), s, metav1.CreateOptions{})
}

// createTarget creates a SlackTarget object initialized with the given options.
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

func withSecret(secretName string) targetOption {
	return func(src *unstructured.Unstructured) {
		slackToken := map[string]interface{}{
			"secretKeyRef": map[string]interface{}{
				"name": secretName,
				"key":  "token",
			},
		}

		if err := unstructured.SetNestedField(src.Object, slackToken, "spec", "token"); err != nil {
			framework.FailfWithOffset(3, "Failed to set spec.token field: %s", err)
		}
	}
}

// captureEvents - Listen to events from the webhook, but capture the errors or message being sought
func captureEvents(rtm *slack.RTM, rv chan slack.RTMEvent) {
	for msg := range rtm.IncomingEvents {
		switch msg.Data.(type) {
		case *slack.MessageEvent:
			rv <- msg
		case *slack.RTMError:
			rv <- msg
		case *slack.InvalidAuthEvent:
			rv <- msg
		default:
			// ignore other events (may want to log at some point)
		}
	}
}
