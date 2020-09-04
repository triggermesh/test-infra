/*
Copyright 2016-2020 The Kubernetes Authors.
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

package utils

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"

	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	appsclient "k8s.io/client-go/kubernetes/typed/apps/v1"
)

type LogfFn func(format string, args ...interface{})

func LogReplicaSetsOfDeployment(deployment *apps.Deployment, allOldRSs []*apps.ReplicaSet, newRS *apps.ReplicaSet, logf LogfFn) {
	if newRS != nil {
		logf(spew.Sprintf("New ReplicaSet %q of Deployment %q:\n%+v", newRS.Name, deployment.Name, *newRS))
	} else {
		logf("New ReplicaSet of Deployment %q is nil.", deployment.Name)
	}
	if len(allOldRSs) > 0 {
		logf("All old ReplicaSets of Deployment %q:", deployment.Name)
	}
	for i := range allOldRSs {
		logf(spew.Sprintf("%+v", *allOldRSs[i]))
	}
}

func LogPodsOfDeployment(c clientset.Interface, deployment *apps.Deployment, rsList []*apps.ReplicaSet, logf LogfFn) {
	minReadySeconds := deployment.Spec.MinReadySeconds
	podListFunc := func(namespace string, options metav1.ListOptions) (*v1.PodList, error) {
		return c.CoreV1().Pods(namespace).List(context.TODO(), options)
	}

	podList, err := ListPods(deployment, rsList, podListFunc)
	if err != nil {
		logf("Failed to list Pods of Deployment %q: %v", deployment.Name, err)
		return
	}
	for _, pod := range podList.Items {
		availability := "not available"
		if IsPodAvailable(&pod, minReadySeconds, metav1.Now()) {
			availability = "available"
		}
		logf(spew.Sprintf("Pod %q is %s:\n%+v", pod.Name, availability, pod))
	}
}

// Waits for the deployment to complete.
// If during a rolling update (rolling == true), returns an error if the deployment's
// rolling update strategy (max unavailable or max surge) is broken at any times.
// It's not seen as a rolling update if shortly after a scaling event or the deployment is just created.
func waitForDeploymentCompleteMaybeCheckRolling(c clientset.Interface, d *apps.Deployment, rolling bool, logf LogfFn, pollInterval, pollTimeout time.Duration) error {
	var (
		deployment *apps.Deployment
		reason     string
	)

	err := wait.PollImmediate(pollInterval, pollTimeout, func() (bool, error) {
		var err error
		deployment, err = c.AppsV1().Deployments(d.Namespace).Get(context.TODO(), d.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		// If during a rolling update, make sure rolling update strategy isn't broken at any times.
		if rolling {
			reason, err = checkRollingUpdateStatus(c, deployment, logf)
			if err != nil {
				return false, err
			}
			logf(reason)
		}

		// When the deployment status and its underlying resources reach the desired state, we're done
		if DeploymentComplete(d, &deployment.Status) {
			return true, nil
		}

		reason = fmt.Sprintf("deployment status: %#v", deployment.Status)
		logf(reason)

		return false, nil
	})

	if err == wait.ErrWaitTimeout {
		err = fmt.Errorf("%s", reason)
	}
	if err != nil {
		return fmt.Errorf("error waiting for deployment %q status to match expectation: %v", d.Name, err)
	}
	return nil
}

func checkRollingUpdateStatus(c clientset.Interface, deployment *apps.Deployment, logf LogfFn) (string, error) {
	var reason string
	oldRSs, allOldRSs, newRS, err := GetAllReplicaSets(deployment, c.AppsV1())
	if err != nil {
		return "", err
	}
	if newRS == nil {
		// New RC hasn't been created yet.
		reason = "new replica set hasn't been created yet"
		return reason, nil
	}
	allRSs := append(oldRSs, newRS)
	// The old/new ReplicaSets need to contain the pod-template-hash label
	for i := range allRSs {
		if !SelectorHasLabel(allRSs[i].Spec.Selector, apps.DefaultDeploymentUniqueLabelKey) {
			reason = "all replica sets need to contain the pod-template-hash label"
			return reason, nil
		}
	}

	// Check max surge and min available
	totalCreated := GetReplicaCountForReplicaSets(allRSs)
	maxCreated := *(deployment.Spec.Replicas) + MaxSurge(*deployment)
	if totalCreated > maxCreated {
		LogReplicaSetsOfDeployment(deployment, allOldRSs, newRS, logf)
		LogPodsOfDeployment(c, deployment, allRSs, logf)
		return "", fmt.Errorf("total pods created: %d, more than the max allowed: %d", totalCreated, maxCreated)
	}
	minAvailable := MinAvailable(deployment)
	if deployment.Status.AvailableReplicas < minAvailable {
		LogReplicaSetsOfDeployment(deployment, allOldRSs, newRS, logf)
		LogPodsOfDeployment(c, deployment, allRSs, logf)
		return "", fmt.Errorf("total pods available: %d, less than the min required: %d", deployment.Status.AvailableReplicas, minAvailable)
	}
	return "", nil
}

// Waits for the deployment to complete, and check rolling update strategy isn't broken at any times.
// Rolling update strategy should not be broken during a rolling update.
func WaitForDeploymentCompleteAndCheckRolling(c clientset.Interface, d *apps.Deployment, logf LogfFn, pollInterval, pollTimeout time.Duration) error {
	rolling := true
	return waitForDeploymentCompleteMaybeCheckRolling(c, d, rolling, logf, pollInterval, pollTimeout)
}

// Waits for the deployment to complete, and don't check if rolling update strategy is broken.
// Rolling update strategy is used only during a rolling update, and can be violated in other situations,
// such as shortly after a scaling event or the deployment is just created.
func WaitForDeploymentComplete(c clientset.Interface, d *apps.Deployment, logf LogfFn, pollInterval, pollTimeout time.Duration) error {
	rolling := false
	return waitForDeploymentCompleteMaybeCheckRolling(c, d, rolling, logf, pollInterval, pollTimeout)
}

// WaitForDeploymentRevisionAndImage waits for the deployment's and its new RS's revision and container image to match the given revision and image.
// Note that deployment revision and its new RS revision should be updated shortly, so we only wait for 1 minute here to fail early.
func WaitForDeploymentRevisionAndImage(c clientset.Interface, ns, deploymentName string, revision, image string, logf LogfFn, pollInterval, pollTimeout time.Duration) error {
	var deployment *apps.Deployment
	var newRS *apps.ReplicaSet
	var reason string
	err := wait.PollImmediate(pollInterval, pollTimeout, func() (bool, error) {
		var err error
		deployment, err = c.AppsV1().Deployments(ns).Get(context.TODO(), deploymentName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		// The new ReplicaSet needs to be non-nil and contain the pod-template-hash label
		newRS, err = GetNewReplicaSet(deployment, c.AppsV1())
		if err != nil {
			return false, err
		}
		if err := checkRevisionAndImage(deployment, newRS, revision, image); err != nil {
			reason = err.Error()
			logf(reason)
			return false, nil
		}
		return true, nil
	})
	if err == wait.ErrWaitTimeout {
		LogReplicaSetsOfDeployment(deployment, nil, newRS, logf)
		err = fmt.Errorf(reason)
	}
	if newRS == nil {
		return fmt.Errorf("deployment %q failed to create new replica set", deploymentName)
	}
	if err != nil {
		if deployment == nil {
			return errors.Wrapf(err, "error creating new replica set for deployment %q ", deploymentName)
		}
		deploymentImage := ""
		if len(deployment.Spec.Template.Spec.Containers) > 0 {
			deploymentImage = deployment.Spec.Template.Spec.Containers[0].Image
		}
		newRSImage := ""
		if len(newRS.Spec.Template.Spec.Containers) > 0 {
			newRSImage = newRS.Spec.Template.Spec.Containers[0].Image
		}
		return fmt.Errorf("error waiting for deployment %q (got %s / %s) and new replica set %q (got %s / %s) revision and image to match expectation (expected %s / %s): %v", deploymentName, deployment.Annotations[RevisionAnnotation], deploymentImage, newRS.Name, newRS.Annotations[RevisionAnnotation], newRSImage, revision, image, err)
	}
	return nil
}

// CheckDeploymentRevisionAndImage checks if the input deployment's and its new replica set's revision and image are as expected.
func CheckDeploymentRevisionAndImage(c clientset.Interface, ns, deploymentName, revision, image string) error {
	deployment, err := c.AppsV1().Deployments(ns).Get(context.TODO(), deploymentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("unable to get deployment %s during revision check: %v", deploymentName, err)
	}

	// Check revision of the new replica set of this deployment
	newRS, err := GetNewReplicaSet(deployment, c.AppsV1())
	if err != nil {
		return fmt.Errorf("unable to get new replicaset of deployment %s during revision check: %v", deploymentName, err)
	}
	return checkRevisionAndImage(deployment, newRS, revision, image)
}

func checkRevisionAndImage(deployment *apps.Deployment, newRS *apps.ReplicaSet, revision, image string) error {
	// The new ReplicaSet needs to be non-nil and contain the pod-template-hash label
	if newRS == nil {
		return fmt.Errorf("new replicaset for deployment %q is yet to be created", deployment.Name)
	}
	if !SelectorHasLabel(newRS.Spec.Selector, apps.DefaultDeploymentUniqueLabelKey) {
		return fmt.Errorf("new replica set %q doesn't have %q label selector", newRS.Name, apps.DefaultDeploymentUniqueLabelKey)
	}
	// Check revision of this deployment, and of the new replica set of this deployment
	if deployment.Annotations == nil || deployment.Annotations[RevisionAnnotation] != revision {
		return fmt.Errorf("deployment %q doesn't have the required revision set", deployment.Name)
	}
	if newRS.Annotations == nil || newRS.Annotations[RevisionAnnotation] != revision {
		return fmt.Errorf("new replicaset %q doesn't have the required revision set", newRS.Name)
	}
	// Check the image of this deployment, and of the new replica set of this deployment
	if !containsImage(deployment.Spec.Template.Spec.Containers, image) {
		return fmt.Errorf("deployment %q doesn't have the required image %s set", deployment.Name, image)
	}
	if !containsImage(newRS.Spec.Template.Spec.Containers, image) {
		return fmt.Errorf("new replica set %q doesn't have the required image %s.", newRS.Name, image)
	}
	return nil
}

func containsImage(containers []v1.Container, imageName string) bool {
	for _, container := range containers {
		if container.Image == imageName {
			return true
		}
	}
	return false
}

type UpdateDeploymentFunc func(d *apps.Deployment)

func UpdateDeploymentWithRetries(c clientset.Interface, namespace, name string, applyUpdate UpdateDeploymentFunc, logf LogfFn, pollInterval, pollTimeout time.Duration) (*apps.Deployment, error) {
	var deployment *apps.Deployment
	var updateErr error
	pollErr := wait.PollImmediate(pollInterval, pollTimeout, func() (bool, error) {
		var err error
		if deployment, err = c.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{}); err != nil {
			return false, err
		}
		// Apply the update, then attempt to push it to the apiserver.
		applyUpdate(deployment)
		if deployment, err = c.AppsV1().Deployments(namespace).Update(context.TODO(), deployment, metav1.UpdateOptions{}); err == nil {
			logf("Updating deployment %s", name)
			return true, nil
		}
		updateErr = err
		return false, nil
	})
	if pollErr == wait.ErrWaitTimeout {
		pollErr = fmt.Errorf("couldn't apply the provided updated to deployment %q: %v", name, updateErr)
	}
	return deployment, pollErr
}

func WaitForObservedDeployment(c clientset.Interface, ns, deploymentName string, desiredGeneration int64) error {
	return UtilWaitForObservedDeployment(func() (*apps.Deployment, error) {
		return c.AppsV1().Deployments(ns).Get(context.TODO(), deploymentName, metav1.GetOptions{})
	}, desiredGeneration, 2*time.Second, 1*time.Minute)
}

// WaitForDeploymentRollbackCleared waits for given deployment either started rolling back or doesn't need to rollback.
func WaitForDeploymentRollbackCleared(c clientset.Interface, ns, deploymentName string, pollInterval, pollTimeout time.Duration) error {
	err := wait.PollImmediate(pollInterval, pollTimeout, func() (bool, error) {
		deployment, err := c.AppsV1().Deployments(ns).Get(context.TODO(), deploymentName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		// Rollback not set or is kicked off
		if deployment.Annotations[apps.DeprecatedRollbackTo] == "" {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("error waiting for deployment %s rollbackTo to be cleared: %v", deploymentName, err)
	}
	return nil
}

// WaitForDeploymentUpdatedReplicasGTE waits for given deployment to be observed by the controller and has at least a number of updatedReplicas
func WaitForDeploymentUpdatedReplicasGTE(c clientset.Interface, ns, deploymentName string, minUpdatedReplicas int32, desiredGeneration int64, pollInterval, pollTimeout time.Duration) error {
	var deployment *apps.Deployment
	err := wait.PollImmediate(pollInterval, pollTimeout, func() (bool, error) {
		d, err := c.AppsV1().Deployments(ns).Get(context.TODO(), deploymentName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		deployment = d
		return deployment.Status.ObservedGeneration >= desiredGeneration && deployment.Status.UpdatedReplicas >= minUpdatedReplicas, nil
	})
	if err != nil {
		return fmt.Errorf("error waiting for deployment %q to have at least %d updatedReplicas: %v; latest .status.updatedReplicas: %d", deploymentName, minUpdatedReplicas, err, deployment.Status.UpdatedReplicas)
	}
	return nil
}

func WaitForDeploymentWithCondition(c clientset.Interface, ns, deploymentName, reason string, condType apps.DeploymentConditionType, logf LogfFn, pollInterval, pollTimeout time.Duration) error {
	var deployment *apps.Deployment
	pollErr := wait.PollImmediate(pollInterval, pollTimeout, func() (bool, error) {
		d, err := c.AppsV1().Deployments(ns).Get(context.TODO(), deploymentName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		deployment = d
		cond := GetDeploymentCondition(deployment.Status, condType)
		return cond != nil && cond.Reason == reason, nil
	})
	if pollErr == wait.ErrWaitTimeout {
		pollErr = fmt.Errorf("deployment %q never updated with the desired condition and reason, latest deployment conditions: %+v", deployment.Name, deployment.Status.Conditions)
		_, allOldRSs, newRS, err := GetAllReplicaSets(deployment, c.AppsV1())
		if err == nil {
			LogReplicaSetsOfDeployment(deployment, allOldRSs, newRS, logf)
			LogPodsOfDeployment(c, deployment, append(allOldRSs, newRS), logf)
		}
	}
	return pollErr
}

// Copied from k8s.io/kubernetes/pkg/api/v1/pod to avoid vendoring k8s.io/kubernetes.

const (
	// RevisionAnnotation is the revision annotation of a deployment's replica sets which records its rollout sequence
	RevisionAnnotation = "deployment.kubernetes.io/revision"
)

// GetDeploymentCondition returns the condition with the provided type.
func GetDeploymentCondition(status apps.DeploymentStatus, condType apps.DeploymentConditionType) *apps.DeploymentCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

// IsPodAvailable returns true if a pod is available; false otherwise.
// Precondition for an available pod is that it must be ready. On top
// of that, there are two cases when a pod can be considered available:
// 1. minReadySeconds == 0, or
// 2. LastTransitionTime (is set) + minReadySeconds < current time
func IsPodAvailable(pod *v1.Pod, minReadySeconds int32, now metav1.Time) bool {
	if !IsPodReady(pod) {
		return false
	}

	c := GetPodReadyCondition(pod.Status)
	minReadySecondsDuration := time.Duration(minReadySeconds) * time.Second
	if minReadySeconds == 0 || !c.LastTransitionTime.IsZero() && c.LastTransitionTime.Add(minReadySecondsDuration).Before(now.Time) {
		return true
	}
	return false
}

// IsPodReady returns true if a pod is ready; false otherwise.
func IsPodReady(pod *v1.Pod) bool {
	return IsPodReadyConditionTrue(pod.Status)
}

// IsPodReadyConditionTrue returns true if a pod is ready; false otherwise.
func IsPodReadyConditionTrue(status v1.PodStatus) bool {
	condition := GetPodReadyCondition(status)
	return condition != nil && condition.Status == v1.ConditionTrue
}

// GetPodReadyCondition extracts the pod ready condition from the given status and returns that.
// Returns nil if the condition is not present.
func GetPodReadyCondition(status v1.PodStatus) *v1.PodCondition {
	_, condition := GetPodCondition(&status, v1.PodReady)
	return condition
}

// GetPodCondition extracts the provided condition from the given status and returns that.
// Returns nil and -1 if the condition is not present, and the index of the located condition.
func GetPodCondition(status *v1.PodStatus, conditionType v1.PodConditionType) (int, *v1.PodCondition) {
	if status == nil {
		return -1, nil
	}
	return GetPodConditionFromList(status.Conditions, conditionType)
}

// GetPodConditionFromList extracts the provided condition from the given list of condition and
// returns the index of the condition and the condition. Returns -1 and nil if the condition is not present.
func GetPodConditionFromList(conditions []v1.PodCondition, conditionType v1.PodConditionType) (int, *v1.PodCondition) {
	if conditions == nil {
		return -1, nil
	}
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return i, &conditions[i]
		}
	}
	return -1, nil
}

// Copied from k8s.io/kubernetes/pkg/controller/deployment/util to avoid vendoring k8s.io/kubernetes.

// MaxUnavailable returns the maximum unavailable pods a rolling deployment can take.
func MaxUnavailable(deployment apps.Deployment) int32 {
	if !IsRollingUpdate(&deployment) || *(deployment.Spec.Replicas) == 0 {
		return int32(0)
	}
	// Error caught by validation
	_, maxUnavailable, _ := ResolveFenceposts(deployment.Spec.Strategy.RollingUpdate.MaxSurge, deployment.Spec.Strategy.RollingUpdate.MaxUnavailable, *(deployment.Spec.Replicas))
	if maxUnavailable > *deployment.Spec.Replicas {
		return *deployment.Spec.Replicas
	}
	return maxUnavailable
}

// MinAvailable returns the minimum available pods of a given deployment
func MinAvailable(deployment *apps.Deployment) int32 {
	if !IsRollingUpdate(deployment) {
		return int32(0)
	}
	return *(deployment.Spec.Replicas) - MaxUnavailable(*deployment)
}

// MaxSurge returns the maximum surge pods a rolling deployment can take.
func MaxSurge(deployment apps.Deployment) int32 {
	if !IsRollingUpdate(&deployment) {
		return int32(0)
	}
	// Error caught by validation
	maxSurge, _, _ := ResolveFenceposts(deployment.Spec.Strategy.RollingUpdate.MaxSurge, deployment.Spec.Strategy.RollingUpdate.MaxUnavailable, *(deployment.Spec.Replicas))
	return maxSurge
}

// GetAllReplicaSets returns the old and new replica sets targeted by the given Deployment. It gets PodList and ReplicaSetList from client interface.
// Note that the first set of old replica sets doesn't include the ones with no pods, and the second set of old replica sets include all old replica sets.
// The third returned value is the new replica set, and it may be nil if it doesn't exist yet.
func GetAllReplicaSets(deployment *apps.Deployment, c appsclient.AppsV1Interface) ([]*apps.ReplicaSet, []*apps.ReplicaSet, *apps.ReplicaSet, error) {
	rsList, err := ListReplicaSets(deployment, RsListFromClient(c))
	if err != nil {
		return nil, nil, nil, err
	}
	oldRSes, allOldRSes := FindOldReplicaSets(deployment, rsList)
	newRS := FindNewReplicaSet(deployment, rsList)
	return oldRSes, allOldRSes, newRS, nil
}

// GetNewReplicaSet returns a replica set that matches the intent of the given deployment; get ReplicaSetList from client interface.
// Returns nil if the new replica set doesn't exist yet.
func GetNewReplicaSet(deployment *apps.Deployment, c appsclient.AppsV1Interface) (*apps.ReplicaSet, error) {
	rsList, err := ListReplicaSets(deployment, RsListFromClient(c))
	if err != nil {
		return nil, err
	}
	return FindNewReplicaSet(deployment, rsList), nil
}

// RsListFromClient returns an rsListFunc that wraps the given client.
func RsListFromClient(c appsclient.AppsV1Interface) RsListFunc {
	return func(namespace string, options metav1.ListOptions) ([]*apps.ReplicaSet, error) {
		rsList, err := c.ReplicaSets(namespace).List(context.TODO(), options)
		if err != nil {
			return nil, err
		}
		var ret []*apps.ReplicaSet
		for i := range rsList.Items {
			ret = append(ret, &rsList.Items[i])
		}
		return ret, err
	}
}

// RsListFunc returns the ReplicaSet from the ReplicaSet namespace and the List metav1.ListOptions.
type RsListFunc func(string, metav1.ListOptions) ([]*apps.ReplicaSet, error)

// podListFunc returns the PodList from the Pod namespace and the List metav1.ListOptions.
type podListFunc func(string, metav1.ListOptions) (*v1.PodList, error)

// ListReplicaSets returns a slice of RSes the given deployment targets.
// Note that this does NOT attempt to reconcile ControllerRef (adopt/orphan),
// because only the controller itself should do that.
// However, it does filter out anything whose ControllerRef doesn't match.
func ListReplicaSets(deployment *apps.Deployment, getRSList RsListFunc) ([]*apps.ReplicaSet, error) {
	// TODO: Right now we list replica sets by their labels. We should list them by selector, i.e. the replica set's selector
	//       should be a superset of the deployment's selector, see https://github.com/kubernetes/kubernetes/issues/19830.
	namespace := deployment.Namespace
	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return nil, err
	}
	options := metav1.ListOptions{LabelSelector: selector.String()}
	all, err := getRSList(namespace, options)
	if err != nil {
		return nil, err
	}
	// Only include those whose ControllerRef matches the Deployment.
	owned := make([]*apps.ReplicaSet, 0, len(all))
	for _, rs := range all {
		if metav1.IsControlledBy(rs, deployment) {
			owned = append(owned, rs)
		}
	}
	return owned, nil
}

// ListPods returns a list of pods the given deployment targets.
// This needs a list of ReplicaSets for the Deployment,
// which can be found with ListReplicaSets().
// Note that this does NOT attempt to reconcile ControllerRef (adopt/orphan),
// because only the controller itself should do that.
// However, it does filter out anything whose ControllerRef doesn't match.
func ListPods(deployment *apps.Deployment, rsList []*apps.ReplicaSet, getPodList podListFunc) (*v1.PodList, error) {
	namespace := deployment.Namespace
	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return nil, err
	}
	options := metav1.ListOptions{LabelSelector: selector.String()}
	all, err := getPodList(namespace, options)
	if err != nil {
		return all, err
	}
	// Only include those whose ControllerRef points to a ReplicaSet that is in
	// turn owned by this Deployment.
	rsMap := make(map[types.UID]bool, len(rsList))
	for _, rs := range rsList {
		rsMap[rs.UID] = true
	}
	owned := &v1.PodList{Items: make([]v1.Pod, 0, len(all.Items))}
	for i := range all.Items {
		pod := &all.Items[i]
		controllerRef := metav1.GetControllerOf(pod)
		if controllerRef != nil && rsMap[controllerRef.UID] {
			owned.Items = append(owned.Items, *pod)
		}
	}
	return owned, nil
}

// FindNewReplicaSet returns the new RS this given deployment targets (the one with the same pod template).
func FindNewReplicaSet(deployment *apps.Deployment, rsList []*apps.ReplicaSet) *apps.ReplicaSet {
	sort.Sort(ReplicaSetsByCreationTimestamp(rsList))
	for i := range rsList {
		if EqualIgnoreHash(&rsList[i].Spec.Template, &deployment.Spec.Template) {
			// In rare cases, such as after cluster upgrades, Deployment may end up with
			// having more than one new ReplicaSets that have the same template as its template,
			// see https://github.com/kubernetes/kubernetes/issues/40415
			// We deterministically choose the oldest new ReplicaSet.
			return rsList[i]
		}
	}
	// new ReplicaSet does not exist.
	return nil
}

// EqualIgnoreHash returns true if two given podTemplateSpec are equal, ignoring the diff in value of Labels[pod-template-hash]
// We ignore pod-template-hash because:
// 1. The hash result would be different upon podTemplateSpec API changes
//    (e.g. the addition of a new field will cause the hash code to change)
// 2. The deployment template won't have hash labels
func EqualIgnoreHash(template1, template2 *v1.PodTemplateSpec) bool {
	t1Copy := template1.DeepCopy()
	t2Copy := template2.DeepCopy()
	// Remove hash labels from template.Labels before comparing
	delete(t1Copy.Labels, apps.DefaultDeploymentUniqueLabelKey)
	delete(t2Copy.Labels, apps.DefaultDeploymentUniqueLabelKey)
	return apiequality.Semantic.DeepEqual(t1Copy, t2Copy)
}

// FindOldReplicaSets returns the old replica sets targeted by the given Deployment, with the given slice of RSes.
// Note that the first set of old replica sets doesn't include the ones with no pods, and the second set of old replica sets include all old replica sets.
func FindOldReplicaSets(deployment *apps.Deployment, rsList []*apps.ReplicaSet) ([]*apps.ReplicaSet, []*apps.ReplicaSet) {
	var requiredRSs []*apps.ReplicaSet
	var allRSs []*apps.ReplicaSet
	newRS := FindNewReplicaSet(deployment, rsList)
	for _, rs := range rsList {
		// Filter out new replica set
		if newRS != nil && rs.UID == newRS.UID {
			continue
		}
		allRSs = append(allRSs, rs)
		if *(rs.Spec.Replicas) != 0 {
			requiredRSs = append(requiredRSs, rs)
		}
	}
	return requiredRSs, allRSs
}

// GetReplicaCountForReplicaSets returns the sum of Replicas of the given replica sets.
func GetReplicaCountForReplicaSets(replicaSets []*apps.ReplicaSet) int32 {
	totalReplicas := int32(0)
	for _, rs := range replicaSets {
		if rs != nil {
			totalReplicas += *(rs.Spec.Replicas)
		}
	}
	return totalReplicas
}

// IsRollingUpdate returns true if the strategy type is a rolling update.
func IsRollingUpdate(deployment *apps.Deployment) bool {
	return deployment.Spec.Strategy.Type == apps.RollingUpdateDeploymentStrategyType
}

// DeploymentComplete considers a deployment to be complete once all of its desired replicas
// are updated and available, and no old pods are running.
func DeploymentComplete(deployment *apps.Deployment, newStatus *apps.DeploymentStatus) bool {
	return newStatus.UpdatedReplicas == *(deployment.Spec.Replicas) &&
		newStatus.Replicas == *(deployment.Spec.Replicas) &&
		newStatus.AvailableReplicas == *(deployment.Spec.Replicas) &&
		newStatus.ObservedGeneration >= deployment.Generation
}

// UtilWaitForObservedDeployment polls for deployment to be updated so that deployment.Status.ObservedGeneration >= desiredGeneration.
// Returns error if polling timesout.
func UtilWaitForObservedDeployment(getDeploymentFunc func() (*apps.Deployment, error), desiredGeneration int64, interval, timeout time.Duration) error {
	// TODO: This should take clientset.Interface when all code is updated to use clientset. Keeping it this way allows the function to be used by callers who have client.Interface.
	return wait.PollImmediate(interval, timeout, func() (bool, error) {
		deployment, err := getDeploymentFunc()
		if err != nil {
			return false, err
		}
		return deployment.Status.ObservedGeneration >= desiredGeneration, nil
	})
}

// ResolveFenceposts resolves both maxSurge and maxUnavailable. This needs to happen in one
// step. For example:
//
// 2 desired, max unavailable 1%, surge 0% - should scale old(-1), then new(+1), then old(-1), then new(+1)
// 1 desired, max unavailable 1%, surge 0% - should scale old(-1), then new(+1)
// 2 desired, max unavailable 25%, surge 1% - should scale new(+1), then old(-1), then new(+1), then old(-1)
// 1 desired, max unavailable 25%, surge 1% - should scale new(+1), then old(-1)
// 2 desired, max unavailable 0%, surge 1% - should scale new(+1), then old(-1), then new(+1), then old(-1)
// 1 desired, max unavailable 0%, surge 1% - should scale new(+1), then old(-1)
func ResolveFenceposts(maxSurge, maxUnavailable *intstrutil.IntOrString, desired int32) (int32, int32, error) {
	surge, err := intstrutil.GetValueFromIntOrPercent(intstrutil.ValueOrDefault(maxSurge, intstrutil.FromInt(0)), int(desired), true)
	if err != nil {
		return 0, 0, err
	}
	unavailable, err := intstrutil.GetValueFromIntOrPercent(intstrutil.ValueOrDefault(maxUnavailable, intstrutil.FromInt(0)), int(desired), false)
	if err != nil {
		return 0, 0, err
	}

	if surge == 0 && unavailable == 0 {
		// Validation should never allow the user to explicitly use zero values for both maxSurge
		// maxUnavailable. Due to rounding down maxUnavailable though, it may resolve to zero.
		// If both fenceposts resolve to zero, then we should set maxUnavailable to 1 on the
		// theory that surge might not work due to quota.
		unavailable = 1
	}

	return int32(surge), int32(unavailable), nil
}

// Copied from k8s.io/kubernetes/pkg/controller to avoid vendoring k8s.io/kubernetes.

// ReplicaSetsByCreationTimestamp sorts a list of ReplicaSet by creation timestamp, using their names as a tie breaker.
type ReplicaSetsByCreationTimestamp []*apps.ReplicaSet

func (o ReplicaSetsByCreationTimestamp) Len() int      { return len(o) }
func (o ReplicaSetsByCreationTimestamp) Swap(i, j int) { o[i], o[j] = o[j], o[i] }
func (o ReplicaSetsByCreationTimestamp) Less(i, j int) bool {
	if o[i].CreationTimestamp.Equal(&o[j].CreationTimestamp) {
		return o[i].Name < o[j].Name
	}
	return o[i].CreationTimestamp.Before(&o[j].CreationTimestamp)
}

// Copied from k8s.io/kubernetes/pkg/util/labels to avoid vendoring k8s.io/kubernetes.

// SelectorHasLabel checks if the given selector contains the given label key in its MatchLabels
func SelectorHasLabel(selector *metav1.LabelSelector, labelKey string) bool {
	return len(selector.MatchLabels[labelKey]) > 0
}
