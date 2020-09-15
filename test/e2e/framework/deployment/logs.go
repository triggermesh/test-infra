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

package deployment

import (
	"context"
	"io"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/triggermesh/test-infra/test/e2e/framework"
)

// GetLogs returns a stream of the logs of the first container in one of the
// randomly-chosen Pods managed by the Deployment with the given name. Meant to
// be used with Deployments that have a single replica and a single container.
func GetLogs(cli kubernetes.Interface, namespace, deploymentName string) io.ReadCloser {
	depl, err := cli.AppsV1().Deployments(namespace).Get(context.Background(), deploymentName, metav1.GetOptions{})
	if err != nil {
		framework.FailfWithOffset(2, "Failed to get Deployment: %s", err)
	}

	deplPods := podsForSelector(cli, namespace, depl.Spec.Selector)
	if len(deplPods) == 0 {
		return nil
	}

	pod := deplPods[0]

	logStream, err := cli.CoreV1().Pods(namespace).
		GetLogs(pod.Name, &corev1.PodLogOptions{Container: pod.Spec.Containers[0].Name}).Stream(context.Background())
	if err != nil {
		framework.FailfWithOffset(2, "Failed to stream logs of Pod %s: %s", pod.Name, err)
	}

	return logStream
}

// podsForSelector returns the list of all Pods that match the given LabelSelector.
func podsForSelector(cli kubernetes.Interface, namespace string, s *metav1.LabelSelector) []corev1.Pod {
	selectorStr := metav1.FormatLabelSelector(s)

	pods, err := cli.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{LabelSelector: selectorStr})
	if err != nil {
		framework.FailfWithOffset(3, "Failed to list Pods for selector %q: %s", selectorStr, err)
	}

	return pods.Items
}
