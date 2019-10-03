// Copyright 2019 The Interconnectedcloud Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package framework

import (
	"context"
	"time"

	v1alpha1 "github.com/interconnectedcloud/qdr-operator/pkg/apis/interconnectedcloud/v1alpha1"
	"github.com/interconnectedcloud/qdr-operator/pkg/utils/selectors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	timeout time.Duration = 60 * time.Second
)

// InterconnectCustomizer represents a function that allows for
// customizing an Interconnect resource before it is created.
type InterconnectCustomizer func(interconnect *v1alpha1.Interconnect)

// CreateInterconnect creates an interconnect resource
func (f *Framework) CreateInterconnect(namespace string, size int32, fn ...InterconnectCustomizer) (*v1alpha1.Interconnect, error) {

	obj := &v1alpha1.Interconnect{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Interconnect",
			APIVersion: "interconnectedcloud.github.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.UniqueName,
			Namespace: namespace,
		},
		Spec: v1alpha1.InterconnectSpec{
			DeploymentPlan: v1alpha1.DeploymentPlanType{
				Size:      size,
				Image:     TestContext.QdrImage,
				Role:      "interior",
				Placement: "Any",
			},
		},
	}

	// Customize the interconnect resource before creation
	for _, f := range fn {
		f(obj)
	}
	// create the interconnect resource
	return f.QdrClient.InterconnectedcloudV1alpha1().Interconnects(f.Namespace).Create(obj)
}

func (f *Framework) DeleteInterconnect(interconnect *v1alpha1.Interconnect) error {
	return f.QdrClient.InterconnectedcloudV1alpha1().Interconnects(f.Namespace).Delete(interconnect.Name, &metav1.DeleteOptions{})
}

func (f *Framework) GetInterconnect(name string) (*v1alpha1.Interconnect, error) {
	return f.QdrClient.InterconnectedcloudV1alpha1().Interconnects(f.Namespace).Get(name, metav1.GetOptions{})
}

func (f *Framework) UpdateInterconnect(interconnect *v1alpha1.Interconnect) (*v1alpha1.Interconnect, error) {
	return f.QdrClient.InterconnectedcloudV1alpha1().Interconnects(f.Namespace).Update(interconnect)
}

func (f *Framework) InterconnectHasExpectedSize(interconnect *v1alpha1.Interconnect, expectedSize int) (bool, error) {
	dep, err := f.GetDeployment(interconnect.Name)
	if err != nil {
		return false, err
	}

	if int(dep.Status.AvailableReplicas) == expectedSize {
		return true, nil
	} else {
		return false, nil
	}
}

func (f *Framework) InterconnectHasExpectedVersion(interconnect *v1alpha1.Interconnect, expectedVersion string) (bool, error) {
	pods, err := f.PodsForInterconnect(interconnect)
	if err != nil {
		return false, err
	}

	for _, pod := range pods {
		version, err := f.VersionForPod(pod)
		if err != nil {
			return false, err
		}
		if version != expectedVersion {
			return false, nil
		}
	}
	return true, nil
}

func (f *Framework) PodsForInterconnect(interconnect *v1alpha1.Interconnect) ([]corev1.Pod, error) {
	selector := selectors.ResourcesByInterconnectName(interconnect.Name)
	listOps := metav1.ListOptions{LabelSelector: selector.String()}
	pods, err := f.KubeClient.CoreV1().Pods(interconnect.Namespace).List(listOps)
	if err != nil {
		return nil, err
	}
	return pods.Items, nil
}

func (f *Framework) VersionForPod(pod corev1.Pod) (string, error) {
	command := []string{"qdrouterd", "--version"}
	kubeExec := NewKubectlExecCommand(f, pod.Name, timeout, command...)
	return kubeExec.Exec()
}

// WaitUntilFullInterconnectWithVersion waits until all the pods belonging to
// the Interconnect deployment report the expected state and expected version.
// The expected state will differe for interior versus edge roles
func (f *Framework) WaitUntilFullInterconnectWithVersion(ctx context.Context, interconnect *v1alpha1.Interconnect, expectedSize int, expectedVersion string) error {
	// Wait for the expected size to be reported on the Interconnect deployment
	err := WaitForDeployment(f.KubeClient, f.Namespace, interconnect.Name, expectedSize, RetryInterval, Timeout)
	if err != nil {
		return err
	}

	return RetryWithContext(ctx, 10*time.Second, func() (bool, error) {
		// Check that the Interconnect deployment of the expected size is created
		s, err := f.InterconnectHasExpectedSize(interconnect, expectedSize)
		if err != nil {
			return false, nil
		}
		if !s {
			return false, nil
		}

		// Check whether all pods in the cluster are reporting the expected version
		v, err := f.InterconnectHasExpectedVersion(interconnect, expectedVersion)
		if err != nil {
			return false, nil
		}
		if !v {
			return false, nil
		}
		return true, nil
	})
}
