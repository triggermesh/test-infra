/*
Copyright 2018-2020 The Kubernetes Authors.
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

// TODO: Refactor common part of functions in this file for generic object kinds.

package utils

import (
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientset "k8s.io/client-go/kubernetes"
)

func deleteResource(c clientset.Interface, kind schema.GroupKind, namespace, name string, options metav1.DeleteOptions) error {
	switch kind {
	case CoreKind("Pod"):
		return c.CoreV1().Pods(namespace).Delete(name, &options)
	case CoreKind("ReplicationController"):
		return c.CoreV1().ReplicationControllers(namespace).Delete(name, &options)
	case ExtensionsKind("ReplicaSet"), AppsKind("ReplicaSet"):
		return c.AppsV1().ReplicaSets(namespace).Delete(name, &options)
	case ExtensionsKind("Deployment"), AppsKind("Deployment"):
		return c.AppsV1().Deployments(namespace).Delete(name, &options)
	case ExtensionsKind("DaemonSet"):
		return c.AppsV1().DaemonSets(namespace).Delete(name, &options)
	case BatchKind("Job"):
		return c.BatchV1().Jobs(namespace).Delete(name, &options)
	case CoreKind("Secret"):
		return c.CoreV1().Secrets(namespace).Delete(name, &options)
	case CoreKind("ConfigMap"):
		return c.CoreV1().ConfigMaps(namespace).Delete(name, &options)
	case CoreKind("Service"):
		return c.CoreV1().Services(namespace).Delete(name, &options)
	default:
		return fmt.Errorf("Unsupported kind when deleting: %v", kind)
	}
}

func DeleteResourceWithRetries(c clientset.Interface, kind schema.GroupKind, namespace, name string, options metav1.DeleteOptions) error {
	deleteFunc := func() (bool, error) {
		err := deleteResource(c, kind, namespace, name, options)
		if err == nil || apierrors.IsNotFound(err) {
			return true, nil
		}
		if IsRetryableAPIError(err) {
			return false, nil
		}
		return false, fmt.Errorf("Failed to delete object with non-retriable error: %v", err)
	}
	return RetryWithExponentialBackOff(deleteFunc)
}

// Copied from k8s.io/kubernetes/pkg/apis/core to avoid vendoring k8s.io/kubernetes.

// CoreGroupName is the group name use in this package
const CoreGroupName = ""

// CoreSchemeGroupVersion is group version used to register these objects
var CoreSchemeGroupVersion = schema.GroupVersion{Group: CoreGroupName, Version: runtime.APIVersionInternal}

// CoreKind takes an unqualified kind and returns a Group qualified GroupKind
func CoreKind(kind string) schema.GroupKind {
	return CoreSchemeGroupVersion.WithKind(kind).GroupKind()
}

// CoreResource takes an unqualified resource and returns a Group qualified GroupResource
func CoreResource(resource string) schema.GroupResource {
	return CoreSchemeGroupVersion.WithResource(resource).GroupResource()
}

// Copied from k8s.io/kubernetes/pkg/apis/apps to avoid vendoring k8s.io/kubernetes.

// AppsGroupName is the group name use in this package
const AppsGroupName = "apps"

// AppsSchemeGroupVersion is group version used to register these objects
var AppsSchemeGroupVersion = schema.GroupVersion{Group: AppsGroupName, Version: runtime.APIVersionInternal}

// AppsKind takes an unqualified kind and returns a Group qualified GroupKind
func AppsKind(kind string) schema.GroupKind {
	return AppsSchemeGroupVersion.WithKind(kind).GroupKind()
}

// AppsResource takes an unqualified resource and returns a Group qualified GroupResource
func AppsResource(resource string) schema.GroupResource {
	return AppsSchemeGroupVersion.WithResource(resource).GroupResource()
}

// Copied from k8s.io/kubernetes/pkg/apis/batch to avoid vendoring k8s.io/kubernetes.

// BatchGroupName is the group name use in this package
const BatchGroupName = "batch"

// BatchSchemeGroupVersion is group version used to register these objects
var BatchSchemeGroupVersion = schema.GroupVersion{Group: BatchGroupName, Version: runtime.APIVersionInternal}

// BatchKind takes an unqualified kind and returns a Group qualified GroupKind
func BatchKind(kind string) schema.GroupKind {
	return BatchSchemeGroupVersion.WithKind(kind).GroupKind()
}

// BatchResource takes an unqualified resource and returns a Group qualified GroupResource
func BatchResource(resource string) schema.GroupResource {
	return BatchSchemeGroupVersion.WithResource(resource).GroupResource()
}

// Copied from k8s.io/kubernetes/pkg/apis/extensions to avoid vendoring k8s.io/kubernetes.

// ExtensionsGroupName is the group name use in this package
const ExtensionsGroupName = "extensions"

// ExtensionsSchemeGroupVersion is group version used to register these objects
var ExtensionsSchemeGroupVersion = schema.GroupVersion{Group: ExtensionsGroupName, Version: runtime.APIVersionInternal}

// ExtensionsKind takes an unqualified kind and returns a Group qualified GroupKind
func ExtensionsKind(kind string) schema.GroupKind {
	return ExtensionsSchemeGroupVersion.WithKind(kind).GroupKind()
}

// ExtensionsResource takes an unqualified resource and returns a Group qualified GroupResource
func ExtensionsResource(resource string) schema.GroupResource {
	return ExtensionsSchemeGroupVersion.WithResource(resource).GroupResource()
}
