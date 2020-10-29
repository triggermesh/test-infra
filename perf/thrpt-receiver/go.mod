module thrpt-receiver

go 1.15

// Transitive dependency of knative.dev/pkg
replace k8s.io/client-go => k8s.io/client-go v0.18.8

require (
	github.com/cloudevents/sdk-go/v2 v2.3.1
	github.com/google/mako v0.0.0-20190821191249-122f8dcef9e3
	github.com/sethvargo/go-signalcontext v0.1.0
	knative.dev/pkg v0.0.0-20201029122234-6d905b3f84a6
)
