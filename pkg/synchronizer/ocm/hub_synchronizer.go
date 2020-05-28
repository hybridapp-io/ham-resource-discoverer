// Copyright 2019 The Kubernetes Authors.
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
package ocm

import (
	"github.com/hybridapp-io/ham-resource-discoverer/pkg/synchronizer"
	"github.com/hybridapp-io/ham-resource-discoverer/pkg/utils"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"

	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/apps/v1"
	subv1 "github.com/open-cluster-management/multicloud-operators-subscription/pkg/apis/apps/v1"
)

const (
	packageInfoLogLevel = 3
)

var _ synchronizer.HubSynchronizerInterface = &HubSynchronizer{}

type HubSynchronizer struct {
	Explorer *utils.Explorer
}

func (h *HubSynchronizer) PatchManagedClusterObject(dpl *unstructured.Unstructured,
	metaobj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	klog.V(packageInfoLogLevel).Info("Patching object ", metaobj.GetNamespace()+"/"+metaobj.GetName())

	// if the object is controlled by other deployable, do not change its ownership
	if hostingAnnotation, ok := (metaobj.GetAnnotations()[dplv1.AnnotationHosting]); ok {
		var owner = types.NamespacedName{Namespace: dpl.GetNamespace(), Name: dpl.GetName()}.String()
		if hostingAnnotation != owner {
			klog.V(packageInfoLogLevel).Info("Not changing the ownership of ", metaobj.GetNamespace()+"/"+metaobj.GetName(),
				" to ", owner, " as it is already owned by deployable ", hostingAnnotation)
			return metaobj, nil
		}
	}
	objgvr := h.Explorer.GVKGVRMap[metaobj.GroupVersionKind()]

	ucobj, err := h.Explorer.DynamicMCClient.Resource(objgvr).Namespace(metaobj.GetNamespace()).Get(metaobj.GetName(), metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}

		klog.Error("Failed to patch managed cluster object with error: ", err)

		return nil, err
	}

	annotations := ucobj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[subv1.AnnotationHosting] = "/"
	annotations[subv1.AnnotationSyncSource] = "subnsdpl-/"
	annotations[dplv1.AnnotationHosting] = types.NamespacedName{Namespace: dpl.GetNamespace(), Name: dpl.GetName()}.String()

	ucobj.SetAnnotations(annotations)
	ucobj, err = h.Explorer.DynamicMCClient.Resource(objgvr).Namespace(metaobj.GetNamespace()).Update(ucobj, metav1.UpdateOptions{})
	if err == nil {
		klog.V(packageInfoLogLevel).Info("Successfully patched object ", metaobj.GetNamespace()+"/"+metaobj.GetName())
	}
	return ucobj, err
}

func (h *HubSynchronizer) GetHostingAnnotations() []string {
	return []string{
		dplv1.AnnotationHosting,
		subv1.AnnotationHosting,
	}
}
