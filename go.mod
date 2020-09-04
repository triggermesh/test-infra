module github.com/triggermesh/test-infra

go 1.15

// Dependencies of the e2e testing framework
require (
	github.com/davecgh/go-spew v1.1.1
	github.com/onsi/ginkgo v1.12.1
	github.com/onsi/gomega v1.10.1
	github.com/pkg/errors v0.9.1
	github.com/spf13/viper v1.7.1
	github.com/stretchr/testify v1.5.1
	gopkg.in/yaml.v2 v2.3.0
	k8s.io/api v0.19.0
	k8s.io/apimachinery v0.19.0
	k8s.io/apiserver v0.19.0
	k8s.io/client-go v0.19.0
	k8s.io/component-base v0.19.0
	k8s.io/klog/v2 v2.2.0
	k8s.io/kubectl v0.19.0
	k8s.io/utils v0.0.0-20200729134348-d5654de09c73
)
