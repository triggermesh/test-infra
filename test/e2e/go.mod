module github.com/triggermesh/test-infra/test/e2e

go 1.15

require (
	cloud.google.com/go/pubsub v1.6.1
	cloud.google.com/go/storage v1.10.0
	github.com/aws/aws-sdk-go v1.34.22
	github.com/cloudevents/sdk-go/v2 v2.2.0
	github.com/google/go-github/v32 v32.1.0
	github.com/google/uuid v1.1.1
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.10.2
	github.com/slack-go/slack v0.9.3
	github.com/xanzy/go-gitlab v0.32.0
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	golang.org/x/sys v0.0.0-20210629170331-7dc0b73dc9fb // indirect
	golang.org/x/tools v0.1.4 // indirect
	google.golang.org/api v0.29.0
	k8s.io/api v0.18.8
	k8s.io/apimachinery v0.18.8
	k8s.io/client-go v11.0.1-0.20190805182717-6502b5e7b1b5+incompatible
	knative.dev/eventing v0.17.1-0.20200911213100-a44dbdbbcec5
	knative.dev/pkg v0.0.0-20200915011641-2e7d80578f25
	knative.dev/serving v0.17.1-0.20200915040141-6ca1381819e9
)

// Transitive dependencies of Knative.
replace k8s.io/client-go => k8s.io/client-go v0.18.8
