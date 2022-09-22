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

package utils

import (
	"context"
	"regexp"
	"strings"

	corev1alpha1 "github.com/hybridapp-io/ham-resource-discoverer/pkg/apis/core/v1alpha1"
	subv1 "github.com/open-cluster-management/multicloud-operators-subscription/pkg/apis/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
)

const (
	packageInfoLogLevel = 3
)

func IsInClusterDeployer(deployer *corev1alpha1.Deployer) bool {
	incluster := true

	annotations := deployer.GetAnnotations()
	if annotations != nil {
		if in, ok := annotations[corev1alpha1.DeployerInCluster]; ok && in == "false" {
			incluster = false
		}
	}

	return incluster
}

func SetInClusterDeployer(deployer *corev1alpha1.Deployer) {
	annotations := deployer.GetAnnotations()
	annotations[corev1alpha1.DeployerInCluster] = "true"
	deployer.SetAnnotations(annotations)
}

func SetRemoteDeployer(deployer *corev1alpha1.Deployer) {
	annotations := deployer.GetAnnotations()
	annotations[corev1alpha1.DeployerInCluster] = "false"
	deployer.SetAnnotations(annotations)
}

// StripVersion removes the version part of a GV
func StripVersion(gv string) string {
	if gv == "" {
		return gv
	}

	re := regexp.MustCompile(`^[vV][0-9].*`)
	// If it begins with only version, (group is nil), return empty string which maps to core group
	if re.MatchString(gv) {
		return ""
	}

	return strings.Split(gv, "/")[0]
}

func StripGroup(gv string) string {

	re := regexp.MustCompile(`^[vV][0-9].*`)
	// If it begins with only version, (group is nil), return empty string which maps to core group
	if re.MatchString(gv) {
		return gv
	}

	return strings.Split(gv, "/")[1]
}

func PatchManagedClusterObject(explorer *Explorer, dpl *unstructured.Unstructured,
	metaobj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	klog.V(packageInfoLogLevel).Info("Patching object ", metaobj.GetNamespace()+"/"+metaobj.GetName())

	// if the object is controlled by other deployable, do not change its ownership

	if hostingAnnotation, ok := (metaobj.GetAnnotations()[corev1alpha1.AnnotationHosting]); ok {
		var owner = types.NamespacedName{Namespace: dpl.GetNamespace(), Name: dpl.GetName()}.String()
		if hostingAnnotation != owner {
			klog.V(packageInfoLogLevel).Info("Not changing the ownership of ", metaobj.GetNamespace()+"/"+metaobj.GetName(),
				" to ", owner, " as it is already owned by deployable ", hostingAnnotation)
			return metaobj, nil
		}
	}
	objgvr := explorer.GVKGVRMap[metaobj.GroupVersionKind()]

	ucobj, err := explorer.DynamicMCClient.Resource(objgvr).Namespace(metaobj.GetNamespace()).Get(context.TODO(), metaobj.GetName(), metav1.GetOptions{})
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
	annotations[corev1alpha1.AnnotationHosting] = types.NamespacedName{Namespace: dpl.GetNamespace(), Name: dpl.GetName()}.String()

	ucobj.SetAnnotations(annotations)
	ucobj, err = explorer.DynamicMCClient.Resource(objgvr).Namespace(metaobj.GetNamespace()).Update(context.TODO(), ucobj, metav1.UpdateOptions{})
	if err == nil {
		klog.V(packageInfoLogLevel).Info("Successfully patched object ", metaobj.GetNamespace()+"/"+metaobj.GetName())
	}
	return ucobj, err
}

func GetHostingAnnotations() []string {
	return []string{
		corev1alpha1.AnnotationHosting,
		subv1.AnnotationHosting,
	}
}
