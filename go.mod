module github.com/triggermesh/test-infra

go 1.15

require (
	github.com/aws/aws-sdk-go v1.34.22
	github.com/cloudevents/sdk-go/v2 v2.2.0
	github.com/google/go-github/v32 v32.1.0
	github.com/google/uuid v1.1.1
	github.com/onsi/ginkgo v1.14.1
	github.com/onsi/gomega v1.10.2
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	k8s.io/api v0.18.8
	k8s.io/apimachinery v0.18.8
	k8s.io/client-go v11.0.1-0.20190805182717-6502b5e7b1b5+incompatible
	knative.dev/eventing v0.17.1-0.20200911213100-a44dbdbbcec5
	knative.dev/pkg v0.0.0-20200915011641-2e7d80578f25
	knative.dev/serving v0.17.1-0.20200915040141-6ca1381819e9
)

// Transitive dependencies of Knative.
replace k8s.io/client-go => k8s.io/client-go v0.18.8
