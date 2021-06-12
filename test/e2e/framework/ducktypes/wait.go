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

package ducktypes

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
	watchtools "k8s.io/client-go/tools/watch"

	"knative.dev/pkg/apis"
	"knative.dev/pkg/apis/duck"
	duckv1 "knative.dev/pkg/apis/duck/v1"

	"github.com/triggermesh/test-infra/test/e2e/framework"
)

// WaitUntilReady waits until the given resource's status becomes ready.
func WaitUntilReady(c dynamic.Interface, obj *unstructured.Unstructured) *unstructured.Unstructured {
	return waitUntilReady(c, obj, objectReadyCondition)
}

// WaitUntilAddressable waits until the given resource's addressable URL is accepting requests.
func WaitUntilAddressable(c dynamic.Interface, obj *unstructured.Unstructured) *unstructured.Unstructured {
	return waitUntilReady(c, obj, objectAddressableCondition)
}

// WaitUntilStatusURL waits until the given resource's status.URL is accepting requests.
func WaitUntilStatusURL(c dynamic.Interface, obj *unstructured.Unstructured) *unstructured.Unstructured {
	return waitUntilReady(c, obj, objectStatusAddressCondition)
}

// waitUntilReady waits until the given resource's status becomes ready.
func waitUntilReady(c dynamic.Interface, obj *unstructured.Unstructured, watchCondition objectWatchCondition) *unstructured.Unstructured {
	fieldSelector := fields.OneTermEqualSelector("metadata.name", obj.GetName()).String()
	gvr, _ := meta.UnsafeGuessKindToResource(obj.GroupVersionKind())

	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.FieldSelector = fieldSelector
			return c.Resource(gvr).Namespace(obj.GetNamespace()).List(context.Background(), options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.FieldSelector = fieldSelector
			return c.Resource(gvr).Namespace(obj.GetNamespace()).Watch(context.Background(), options)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	lastEvent, err := watchtools.UntilWithSync(ctx, lw, obj, nil, watchCondition(gvr, obj))
	if err != nil {
		framework.FailfWithOffset(2, "Error waiting for resource %s %q to become ready: %s",
			gvr.GroupResource(), obj.GetName(), err)
	}

	return lastEvent.Object.(*unstructured.Unstructured)
}

type objectWatchCondition func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured) watchtools.ConditionFunc

// objectReadyCondition checks whether the object referenced in the given watch.Event has
// its Ready condition set to True.
func objectReadyCondition(gvr schema.GroupVersionResource, obj *unstructured.Unstructured) watchtools.ConditionFunc {
	return func(e watch.Event) (bool, error) {
		if e.Type == watch.Deleted {
			return false, apierrors.NewNotFound(gvr.GroupResource(), obj.GetName())
		}

		if u, ok := e.Object.(*unstructured.Unstructured); ok {
			res := &duckv1.KResource{}
			if err := duck.FromUnstructured(u, res); err != nil {
				framework.FailfWithOffset(2, "Failed to convert unstructured object to KResource: %s", err)
			}

			if cond := res.Status.GetCondition(apis.ConditionReady); cond != nil && cond.IsTrue() {
				return true, nil
			}
		}

		return false, nil
	}
}

// objectAddressableCondition checks whether the object referenced in the given watch.Event has
// its Ready condition, is addressable and the address is reachable.
func objectAddressableCondition(gvr schema.GroupVersionResource, obj *unstructured.Unstructured) watchtools.ConditionFunc {
	return func(e watch.Event) (bool, error) {
		if e.Type == watch.Deleted {
			return false, apierrors.NewNotFound(gvr.GroupResource(), obj.GetName())
		}

		if u, ok := e.Object.(*unstructured.Unstructured); ok {
			url := AddressOrNil(u)
			if url == nil {
				return false, nil
			}

			if ok, err := urlReadyCondition(url); !ok {
				return false, err
			}

			res := &duckv1.KResource{}
			if err := duck.FromUnstructured(u, res); err != nil {
				framework.FailfWithOffset(2, "Failed to convert unstructured object to KResource: %s", err)
			}

			if cond := res.Status.GetCondition(apis.ConditionReady); cond != nil && cond.IsTrue() {
				return true, nil
			}
		}
		return false, nil
	}
}

// objectStatusAddressCondition checks whether the object referenced in the given watch.Event has
// its Ready condition, contains Status.Address and the address is reachable.
func objectStatusAddressCondition(gvr schema.GroupVersionResource, obj *unstructured.Unstructured) watchtools.ConditionFunc {
	return func(e watch.Event) (bool, error) {
		if e.Type == watch.Deleted {
			return false, apierrors.NewNotFound(gvr.GroupResource(), obj.GetName())
		}

		u, ok := e.Object.(*unstructured.Unstructured)
		if !ok {
			return false, nil
		}

		st, ok := u.Object["status"]
		if !ok {
			framework.Logf("no status yet")
			return false, nil
		}

		status, ok := st.(map[string]interface{})
		if !ok {
			framework.Logf("cannot convert status into map[string]interface{}")
			return false, nil
		}

		su, ok := status["url"]
		if !ok {
			framework.Logf("no status.url yet")
			return false, nil
		}

		ustr, ok := su.(string)
		if !ok {
			framework.Logf("status.url is not a string")
			return false, nil
		}

		url, err := url.Parse(ustr)
		if err != nil {
			framework.FailfWithOffset(2, "Failed to convert unstructured object to KResource: %s", err)
		}

		if ok, err := urlReadyCondition(url); !ok {
			return false, err
		}

		res := &duckv1.KResource{}
		if err := duck.FromUnstructured(u, res); err != nil {
			framework.FailfWithOffset(2, "Failed to convert unstructured object to KResource: %s", err)
		}

		if cond := res.Status.GetCondition(apis.ConditionReady); cond != nil && cond.IsTrue() {
			return true, nil
		}

		return false, nil
	}
}

// urlReadyCondition checks whether the URL is accepting requests
func urlReadyCondition(url *url.URL) (bool, error) {
	port := url.Port()
	if port == "" {
		switch url.Scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		default:
			return false, fmt.Errorf("unsupported URL schema %q", url.Scheme)
		}
	}
	conn, err := net.Dial("tcp", net.JoinHostPort(url.Host, port))
	if err != nil {
		return false, nil
	}
	conn.Close()

	return true, nil
}
