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

package githubsqs

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"

	"github.com/google/go-github/v32/github"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/aws-sdk-go/service/sqs/sqsiface"

	"github.com/triggermesh/test-infra/test/e2e/framework"
	e2esqs "github.com/triggermesh/test-infra/test/e2e/framework/aws/sqs"
	"github.com/triggermesh/test-infra/test/e2e/framework/bridges"
	"github.com/triggermesh/test-infra/test/e2e/framework/ducktypes"
	e2egithub "github.com/triggermesh/test-infra/test/e2e/framework/github"
	"github.com/triggermesh/test-infra/test/e2e/framework/manifest"
)

/* This test suite requires:

- A GitHub OAuth2 access token exported in the environment as GITHUB_API_TOKEN, with the following OAuth scopes: [repo, delete_repo]
- AWS credentials in whichever form (https://docs.aws.amazon.com/sdk-for-go/api/aws/session/#hdr-Sessions_options_from_Shared_Config)
- The name of an AWS region exported in the environment as AWS_REGION
*/

var bridgeAPIVersion = schema.GroupVersion{
	Group:   "flow.triggermesh.io",
	Version: "v1alpha1",
}

const bridgeResource = "bridges"

const ghApiTokenSecretKey = "apiToken"

const awsAccessKeyIDSecretKey = "access_key_id"
const awsSecretAccessKeySecretKey = "secret_access_key"

var _ = Describe("GitHub to SQS", func() {
	f := framework.New("github-sqs")

	var ns string

	var ghClient *github.Client
	var repo *github.Repository

	var sqsClient sqsiface.SQSAPI
	var queueURL string

	BeforeEach(func() {
		var brdgClient dynamic.ResourceInterface
		var ghSecret *corev1.Secret
		var awsSecret *corev1.Secret

		ns = f.UniqueName

		gvr := bridgeAPIVersion.WithResource(bridgeResource)
		brdgClient = f.DynamicClient.Resource(gvr).Namespace(ns)

		ghClient = e2egithub.NewClient()

		sess := session.Must(session.NewSession())
		sqsClient = sqs.New(sess)

		By("creating a GitHub repository", func() {
			repo = e2egithub.CreateRepository(ghClient, f)
		})

		By("creating a SQS queue", func() {
			queueURL = e2esqs.CreateQueue(sqsClient, f)
		})

		By("creating a Kubernetes Secret containing the GitHub API token", func() {
			ghSecret = createAPITokenSecret(f.KubeClient, ns, ghApiTokenSecretKey, e2egithub.APIToken())
		})

		By("creating a Kubernetes Secret containing the AWS Access Credentials", func() {
			awsSecret = createAWSCredsSecret(f.KubeClient, ns, readAWSCredentials(sess))
		})

		By("creating a Bridge object", func() {
			brdgTmpl := manifest.ObjectFromFile("bridges/manifests/github-sqs-bridge.yaml")

			brdg, err := createBridge(brdgClient, brdgTmpl, ns, "test-",
				withRepo(ownerAndRepo(repo)),
				withAPITokenSecret(ghSecret.Name, ghApiTokenSecretKey),
				withARN(e2esqs.QueueARN(sqsClient, queueURL)),
				withAWSCredsSecret(awsSecret.Name),
			)
			Expect(err).ToNot(HaveOccurred())

			ducktypes.WaitUntilReady(f.DynamicClient, brdg)
		})
	})

	AfterEach(func() {
		By("deleting GitHub repository "+ownerAndRepo(repo), func() {
			e2egithub.DeleteRepository(ghClient, *repo.Owner.Login, *repo.Name)
		})

		By("deleting SQS queue "+queueURL, func() {
			e2esqs.DeleteQueue(sqsClient, queueURL)
		})
	})

	It("github push event is sent to SQS queue", func() {
		var commit *github.Commit
		var receivedMsg []byte

		By("creating a Git commit", func() {
			commit = e2egithub.CreateCommit(ghClient, *repo.Owner.Login, *repo.Name)
		})

		By("polling the SQS queue", func() {
			var receivedMsgs []*sqs.Message

			receivedMsgs = e2esqs.ReceiveMessages(sqsClient, queueURL)

			Expect(receivedMsgs).To(HaveLen(1),
				"Received %d messages instead of 1", len(receivedMsgs))

			receivedMsg = []byte(*receivedMsgs[0].Body)
		})

		By("inspecting the message payload", func() {
			msgData := make(map[string]interface{})
			err := json.Unmarshal(receivedMsg, &msgData)
			Expect(err).ToNot(HaveOccurred())

			Expect(msgData["type"]).To(Equal("dev.knative.source.github.push"))

			eventData, err := json.Marshal(msgData["data"])
			Expect(err).ToNot(HaveOccurred())

			ghEvent := &github.PushEvent{}
			err = json.Unmarshal(eventData, ghEvent)
			Expect(err).ToNot(HaveOccurred())

			Expect(ghEvent.GetHeadCommit().GetID()).To(Equal(commit.GetSHA()))
			Expect(ghEvent.GetHeadCommit().GetURL()).To(Equal(commit.GetHTMLURL()))
		})
	})
})

// ownerAndRepo returns a reference to a GitHub repository in the format
// "owner/repo".
func ownerAndRepo(r *github.Repository) string {
	return *r.Owner.Login + "/" + *r.Name
}

// createBridge creates a Bridge object initialized with the given options.
func createBridge(brdgCli dynamic.ResourceInterface, bridge *unstructured.Unstructured,
	namespace, namePrefix string, opts ...bridgeOption) (*unstructured.Unstructured, error) {

	bridge.SetNamespace(namespace)
	bridge.SetGenerateName(namePrefix)

	for _, opt := range opts {
		opt(bridge)
	}

	return brdgCli.Create(context.Background(), bridge, metav1.CreateOptions{})
}

type bridgeOption func(*unstructured.Unstructured)

// withRepo sets the ownerAndRepo spec field of the GitHubSource.
func withRepo(ownerAndRepo string) bridgeOption {
	return func(brdg *unstructured.Unstructured) {
		comps := bridges.Components(brdg)
		ghSrc := comps[bridges.SeekComponentByKind(comps, "GitHubSource")]

		if err := unstructured.SetNestedField(ghSrc, ownerAndRepo, "object", "spec", "ownerAndRepository"); err != nil {
			framework.FailfWithOffset(2, "Failed to set spec.ownerAndRepo field: %s", err)
		}

		// "comps" is a deep copy returned by unstructured.NestedSlice,
		// so we need set the modified version on the Bridge object
		bridges.SetComponents(brdg, comps)
	}
}

// withAPITokenSecret sets the accessToken and secretToken spec fields of the GitHubSource.
func withAPITokenSecret(secretName, apiTokenKey string) bridgeOption {
	return func(brdg *unstructured.Unstructured) {
		comps := bridges.Components(brdg)
		ghSrc := comps[bridges.SeekComponentByKind(comps, "GitHubSource")]

		tokenSecretRef := map[string]interface{}{
			"secretKeyRef": map[string]interface{}{
				"name": secretName,
				"key":  apiTokenKey,
			},
		}

		if err := unstructured.SetNestedMap(ghSrc, tokenSecretRef, "object", "spec", "accessToken"); err != nil {
			framework.FailfWithOffset(2, "Failed to set spec.accessToken field: %s", err)
		}
		if err := unstructured.SetNestedMap(ghSrc, tokenSecretRef, "object", "spec", "secretToken"); err != nil {
			framework.FailfWithOffset(2, "Failed to set spec.secretToken field: %s", err)
		}

		// "comps" is a deep copy returned by unstructured.NestedSlice,
		// so we need set the modified version on the Bridge object
		bridges.SetComponents(brdg, comps)
	}
}

// withARN sets the arn spec field of the AWSSQSTarget.
func withARN(arn string) bridgeOption {
	return func(brdg *unstructured.Unstructured) {
		comps := bridges.Components(brdg)
		sqsTarget := comps[bridges.SeekComponentByKind(comps, "AWSSQSTarget")]

		if err := unstructured.SetNestedField(sqsTarget, arn, "object", "spec", "arn"); err != nil {
			framework.FailfWithOffset(2, "Failed to set spec.arn field: %s", err)
		}

		// "comps" is a deep copy returned by unstructured.NestedSlice,
		// so we need set the modified version on the Bridge object
		bridges.SetComponents(brdg, comps)
	}
}

// withAWSCredsSecret sets the awsApiKey and awsApiSecret spec fields of the AWSSQSTarget.
func withAWSCredsSecret(secretName string) bridgeOption {
	return func(brdg *unstructured.Unstructured) {
		comps := bridges.Components(brdg)
		sqsTarget := comps[bridges.SeekComponentByKind(comps, "AWSSQSTarget")]

		apiKeySecretRef := map[string]interface{}{
			"secretKeyRef": map[string]interface{}{
				"name": secretName,
				"key":  awsAccessKeyIDSecretKey,
			},
		}

		apiSecretSecretRef := map[string]interface{}{
			"secretKeyRef": map[string]interface{}{
				"name": secretName,
				"key":  awsSecretAccessKeySecretKey,
			},
		}

		if err := unstructured.SetNestedMap(sqsTarget, apiKeySecretRef, "object", "spec", "awsApiKey"); err != nil {
			framework.FailfWithOffset(2, "Failed to set spec.accessToken field: %s", err)
		}
		if err := unstructured.SetNestedMap(sqsTarget, apiSecretSecretRef, "object", "spec", "awsApiSecret"); err != nil {
			framework.FailfWithOffset(2, "Failed to set spec.secretToken field: %s", err)
		}

		// "comps" is a deep copy returned by unstructured.NestedSlice,
		// so we need set the modified version on the Bridge object
		bridges.SetComponents(brdg, comps)
	}
}

// createAPITokenSecret creates a Kubernetes Secret containing a GitHub API token.
func createAPITokenSecret(c clientset.Interface, namespace, tokenKey, tokenVal string) *corev1.Secret {
	secr := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    namespace,
			GenerateName: "gh-apitoken-",
		},
		StringData: map[string]string{
			tokenKey: tokenVal,
		},
	}

	var err error

	secr, err = c.CoreV1().Secrets(namespace).Create(context.Background(), secr, metav1.CreateOptions{})
	if err != nil {
		framework.FailfWithOffset(2, "Failed to create Secret: %s", err)
	}

	return secr
}

func readAWSCredentials(sess *session.Session) credentials.Value {
	creds, err := sess.Config.Credentials.Get()
	if err != nil {
		framework.FailfWithOffset(2, "Error reading AWS credentials: %s", err)
	}

	return creds
}

// createAWSCredsSecret creates a Kubernetes Secret containing a AWS credentials.
func createAWSCredsSecret(c clientset.Interface, namespace string, creds credentials.Value) *corev1.Secret {
	secr := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    namespace,
			GenerateName: "aws-creds-",
		},
		StringData: map[string]string{
			awsAccessKeyIDSecretKey:     creds.AccessKeyID,
			awsSecretAccessKeySecretKey: creds.SecretAccessKey,
		},
	}

	var err error

	secr, err = c.CoreV1().Secrets(namespace).Create(context.Background(), secr, metav1.CreateOptions{})
	if err != nil {
		framework.FailfWithOffset(2, "Failed to create Secret: %s", err)
	}

	return secr
}
