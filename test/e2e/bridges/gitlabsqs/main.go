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

package gitlabsqs

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientset "k8s.io/client-go/kubernetes"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/aws-sdk-go/service/sqs/sqsiface"

	gogitlab "github.com/xanzy/go-gitlab"

	"github.com/triggermesh/test-infra/test/e2e/framework"
	e2esqs "github.com/triggermesh/test-infra/test/e2e/framework/aws/sqs"
	"github.com/triggermesh/test-infra/test/e2e/framework/bridges"
	"github.com/triggermesh/test-infra/test/e2e/framework/ducktypes"
	"github.com/triggermesh/test-infra/test/e2e/framework/gitlab"
	"github.com/triggermesh/test-infra/test/e2e/framework/manifest"
)

/* This test suite requires, in addition to AWS and KUBERNETES variables:
 *   - GITLAB_API_TOKEN with the api scope set
 */

var _ = Describe("GitLab to SQS", func() {
	f := framework.New("gitlab-sqs")
	var ns string

	var client *gitlab.GitlabHandle
	var gitlabProject *gogitlab.Project

	var err error

	var gitlabSecret, awsSecret *corev1.Secret

	var sqsClient sqsiface.SQSAPI
	var sqsQueueURL string

	BeforeEach(func() {
		ns = f.UniqueName
		token := gitlab.GetToken()
		gvr := bridgeAPIVersion.WithResource("bridges")
		bridgeClient := f.DynamicClient.Resource(gvr).Namespace(ns)

		awsSession := session.Must(session.NewSession())

		By("creating a new SQS client", func() {
			sqsClient = sqs.New(awsSession)
		})

		By("creating a Kubernetes secret for AWS", func() {
			creds, err := awsSession.Config.Credentials.Get()
			Expect(err).ToNot(HaveOccurred())

			kvMap := make(map[string]string)
			kvMap["access_key_id"] = creds.AccessKeyID
			kvMap["secret_access_key"] = creds.SecretAccessKey

			awsSecret, err = CreateSecret(f.KubeClient, ns, "aws-secret", kvMap)
			Expect(err).ToNot(HaveOccurred())
		})

		By("creating a SQS Queue", func() {
			sqsQueueURL = e2esqs.CreateQueue(sqsClient, f)
		})

		By("creating a new GitLab client", func() {
			client, err = gitlab.NewClient(token)

			if err != nil {
				e2esqs.DeleteQueue(sqsClient, sqsQueueURL)
				framework.FailfWithOffset(2, "Failed to create gitlab client: %s", err)
			}
		})

		By("creating a new GitLab project", func() {
			gitlabProject, err = client.CreateProject(f)
			if err != nil {
				e2esqs.DeleteQueue(sqsClient, sqsQueueURL)
				framework.FailfWithOffset(2, "Failed to create gitlab project: %s", err)
			}
		})

		By("creating a Kubernetes secret for the GitLab API token", func() {
			kvMap := make(map[string]string)
			kvMap["accessToken"] = token
			kvMap["secretToken"] = gitlab.DefaultSecretToken

			gitlabSecret, err = CreateSecret(f.KubeClient, ns, "gl-secret", kvMap)
			if err != nil {
				// Cleanup generated project
				_ = client.DeleteProject(gitlabProject)
				e2esqs.DeleteQueue(sqsClient, sqsQueueURL)
				framework.FailfWithOffset(2, "Failed to create gitlab secret: %s", err)
			}
		})

		By("creating a gitlab->sqs bridge", func() {
			bridgeTemplate := manifest.ObjectFromFile("bridges/manifests/gitlab-sqs-bridge.yaml")
			bridge, err := bridges.CreateBridge(
				bridgeClient,
				bridgeTemplate,
				ns,
				"test-",
				withProject(gitlab.DefaultBaseURL+ns),
				withGitlabCredentials(gitlabSecret),
				withAwsARN(e2esqs.QueueARN(sqsClient, sqsQueueURL)),
				withAwsCredentials(awsSecret))

			if err != nil {
				// Cleanup generated project
				_ = client.DeleteProject(gitlabProject)
				e2esqs.DeleteQueue(sqsClient, sqsQueueURL)
				framework.FailfWithOffset(2, "Failed to create bridge: %s", err)
			}

			ducktypes.WaitUntilReady(f.DynamicClient, bridge)
		})
	})

	AfterEach(func() {
		By("deleting GitLab project "+gitlabProject.Name, func() {
			err = client.DeleteProject(gitlabProject)
			Expect(err).ToNot(HaveOccurred())
		})

		By("deleting AWS SQS Queue", func() {
			e2esqs.DeleteQueue(sqsClient, sqsQueueURL)
		})
	})

	It("creates a new gitlab push event", func() {
		var file *gogitlab.File
		var payload []byte

		By("creating a file", func() {
			file = client.CreateCommit(gitlabProject)
		})

		By("polling the SQS queue", func() {
			receivedMessages := e2esqs.ReceiveMessages(sqsClient, sqsQueueURL)

			Expect(receivedMessages).To(HaveLen(1))
			payload = []byte(*receivedMessages[0].Body)
		})

		By("inspecting the event", func() {
			msgData := make(map[string]interface{})
			err := json.Unmarshal(payload, &msgData)
			Expect(err).ToNot(HaveOccurred())

			Expect(msgData["type"]).To(Equal("dev.knative.sources.gitlab.push"))

			eventData, err := json.Marshal(msgData["data"])
			Expect(err).ToNot(HaveOccurred())

			gitlabEvent := &gogitlab.PushEvent{}
			err = json.Unmarshal(eventData, gitlabEvent)
			Expect(err).ToNot(HaveOccurred())

			Expect(gitlabEvent.ProjectID).To(Equal(gitlabProject.ID))
			Expect(gitlabEvent.TotalCommitsCount).To(Equal(1))
			Expect(gitlabEvent.Commits[0].ID).To(Equal(file.CommitID))
		})
	})
})

func withProject(projectUrl string) bridges.BridgeOption {
	return func(bridge *unstructured.Unstructured) {
		components := bridges.Components(bridge)
		src := components[bridges.SeekComponentByKind(components, "GitLabSource")]

		if err := unstructured.SetNestedField(src, projectUrl, "object", "spec", "projectUrl"); err != nil {
			framework.FailfWithOffset(2, "Failed to set object.spec.projectUrl")
		}

		bridges.SetComponents(bridge, components)
	}
}

func withAwsARN(arn string) bridges.BridgeOption {
	return func(bridge *unstructured.Unstructured) {
		components := bridges.Components(bridge)
		src := components[bridges.SeekComponentByKind(components, "AWSSQSTarget")]

		if err := unstructured.SetNestedField(src, arn, "object", "spec", "arn"); err != nil {
			framework.FailfWithOffset(2, "Failed to set object.spec.arn")
		}

		bridges.SetComponents(bridge, components)
	}
}

func withGitlabCredentials(secret *corev1.Secret) bridges.BridgeOption {
	return func(bridge *unstructured.Unstructured) {
		components := bridges.Components(bridge)
		src := components[bridges.SeekComponentByKind(components, "GitLabSource")]

		accessTokenRef := map[string]interface{}{
			"secretKeyRef": map[string]interface{}{
				"name": secret.Name,
				"key":  "accessToken",
			},
		}

		secretTokenRef := map[string]interface{}{
			"secretKeyRef": map[string]interface{}{
				"name": secret.Name,
				"key":  "secretToken",
			},
		}

		if err := unstructured.SetNestedMap(src, accessTokenRef, "object", "spec", "accessToken"); err != nil {
			framework.FailfWithOffset(2, "Failed to set object.spec.accessToken")
		}

		if err := unstructured.SetNestedMap(src, secretTokenRef, "object", "spec", "secretToken"); err != nil {
			framework.FailfWithOffset(2, "Failed to set object.spec.secretToken")
		}

		bridges.SetComponents(bridge, components)
	}
}

func withAwsCredentials(secret *corev1.Secret) bridges.BridgeOption {
	return func(bridge *unstructured.Unstructured) {
		components := bridges.Components(bridge)
		src := components[bridges.SeekComponentByKind(components, "AWSSQSTarget")]

		awsApiKey := map[string]interface{}{
			"secretKeyRef": map[string]interface{}{
				"name": secret.Name,
				"key":  "access_key_id",
			},
		}

		awsApiSecret := map[string]interface{}{
			"secretKeyRef": map[string]interface{}{
				"name": secret.Name,
				"key":  "secret_access_key",
			},
		}

		if err := unstructured.SetNestedMap(src, awsApiKey, "object", "spec", "awsApiKey"); err != nil {
			framework.FailfWithOffset(2, "Failed to set object.spec.awsApiKey")
		}

		if err := unstructured.SetNestedMap(src, awsApiSecret, "object", "spec", "awsApiSecret"); err != nil {
			framework.FailfWithOffset(2, "Failed to set object.spec.awsApiSecret")
		}

		bridges.SetComponents(bridge, components)
	}
}

// TODO: These should be extracted out into a common location where they could be leveraged by all bridges

// CreateSecret Generate the secret to use and post it to the kubernetes cluster
func CreateSecret(c clientset.Interface, namespace, namePrefix string, kvmap map[string]string) (*corev1.Secret, error) {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    namespace,
			GenerateName: namePrefix,
		},
		StringData: kvmap,
	}
	return c.CoreV1().Secrets(namespace).Create(context.Background(), s, metav1.CreateOptions{})
}

var bridgeAPIVersion = schema.GroupVersion{
	Group:   "flow.triggermesh.io",
	Version: "v1alpha1",
}
