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

package deployable

import (
	"reflect"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"

	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/apps/v1"
	subv1 "github.com/open-cluster-management/multicloud-operators-subscription/pkg/apis/apps/v1"

	hdplv1alpha1 "github.com/hybridapp-io/ham-deployable-operator/pkg/apis/core/v1alpha1"
	corev1alpha1 "github.com/hybridapp-io/ham-resource-discoverer/pkg/apis/core/v1alpha1"
	"github.com/hybridapp-io/ham-resource-discoverer/pkg/utils"
)

const (
	trueCondition       = "true"
	packageInfoLogLevel = 3
)

var (
	ignoreAnnotation = []string{
		dplv1.AnnotationHosting,
		subv1.AnnotationHosting,
	}
)

func SyncDeployable(metaobj *unstructured.Unstructured, explorer *utils.Explorer) error {

	annotations := metaobj.GetAnnotations()
	if annotations != nil {
		matched := false
		for _, key := range ignoreAnnotation {
			if _, matched = annotations[key]; matched {
				break
			}
		}

		if matched {
			klog.Info("Ignore object:", metaobj.GetNamespace(), "/", metaobj.GetName())
			return nil
		}
	}

	dpl, err := locateDeployableForObject(metaobj, explorer)
	if err != nil {
		klog.Error("Failed to locate deployable ", metaobj.GetNamespace()+"/"+metaobj.GetName(), " with error: ", err)
		return err
	}

	if dpl == nil {
		dpl = &dplv1.Deployable{}
		dpl.GenerateName = strings.ToLower(metaobj.GetKind()+"-"+metaobj.GetNamespace()+"-"+metaobj.GetName()) + "-"
		dpl.Namespace = explorer.Cluster.Namespace
	}

	if err = updateDeployableAndObject(dpl, metaobj, explorer); err != nil {
		klog.Error("Failed to update deployable :", metaobj.GetNamespace(), "/", metaobj.GetName()+" with error: ", err)
		return err
	}
	return nil
}

func updateDeployableAndObject(dpl *dplv1.Deployable, metaobj *unstructured.Unstructured, explorer *utils.Explorer) error {

	if dpl.UID == "" {
		refreshedDpl := prepareDeployable(dpl, metaobj, explorer)
		prepareTemplate(metaobj)
		refreshedDpl.Spec.Template = &runtime.RawExtension{
			Object: metaobj,
		}
		ucContent, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(refreshedDpl)

		uc := &unstructured.Unstructured{}
		uc.SetUnstructuredContent(ucContent)
		uc.SetGroupVersionKind(deployableGVK)

		uc, err := explorer.DynamicHubClient.Resource(explorer.GVKGVRMap[deployableGVK]).Namespace(refreshedDpl.Namespace).Create(uc, metav1.CreateOptions{})
		if err != nil {
			klog.Error("Failed to sync deployable ", dpl.Namespace+"/"+dpl.Name)
			return err
		}
		klog.V(packageInfoLogLevel).Info("Successfully added deployable ", uc.GetNamespace()+"/"+uc.GetName(),
			" for object ", metaobj.GetNamespace()+"/"+metaobj.GetName())

	} else if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		uc, err := explorer.DynamicHubClient.Resource(explorer.GVKGVRMap[deployableGVK]).Namespace(dpl.Namespace).Get(dpl.Name, metav1.GetOptions{})
		if err != nil {
			klog.Error("Failed to retrieve deployable from hub ", dpl.Namespace+"/"+dpl.Name)
			return err
		}

		refreshedObject, err := patchObject(uc, metaobj, explorer)
		if err != nil {
			klog.Error("Failed to patch object ", metaobj.GetNamespace()+"/"+metaobj.GetName(), " with error: ", err)
			return err
		}
		// avoid expensive reconciliation logic if no changes in the object structure
		if !reflect.DeepEqual(refreshedObject, metaobj) {
			klog.Info("Updating deployable ", uc.GetNamespace()+"/"+uc.GetName())
			if err = unstructured.SetNestedMap(uc.Object, metaobj.Object, "spec", "template"); err != nil {
				klog.Error("Failed to set the spec template for deployable ", dpl.Namespace+"/"+dpl.Name)
				return err
			}
			// update the hybrid-discovery annotation
			annotations := uc.GetAnnotations()
			annotations[hdplv1alpha1.AnnotationHybridDiscovery] = hdplv1alpha1.HybridDiscoveryCompleted
			uc.SetAnnotations(annotations)

			uc.SetGroupVersionKind(deployableGVK)
			uc, err = explorer.DynamicHubClient.Resource(explorer.GVKGVRMap[deployableGVK]).Namespace(dpl.Namespace).Update(uc, metav1.UpdateOptions{})
			if err != nil {
				klog.Error("Failed to update deployable ", dpl.Namespace+"/"+dpl.Name, ". Retrying... ")
				return err
			}
			klog.V(packageInfoLogLevel).Info("Successfully updated deployable ", uc.GetNamespace()+"/"+uc.GetName(),
				" for object ", metaobj.GetNamespace()+"/"+metaobj.GetName())
		} else {
			klog.V(packageInfoLogLevel).Info("Skipping deployable ", dpl.Namespace+"/"+dpl.Name, ". No changes detected")
		}
		return nil
	}); err != nil {
		klog.Error("Failed to sync deployable ", dpl.Namespace+"/"+dpl.Name)
		return err
	}
	return nil
}

func prepareDeployable(deployable *dplv1.Deployable, metaobj *unstructured.Unstructured, explorer *utils.Explorer) *dplv1.Deployable {
	dpl := deployable.DeepCopy()

	labels := dpl.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	for key, value := range metaobj.GetLabels() {
		labels[key] = value
	}

	dpl.SetLabels(labels)

	annotations := dpl.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[corev1alpha1.SourceObject] = types.NamespacedName{Namespace: metaobj.GetNamespace(), Name: metaobj.GetName()}.String()
	annotations[dplv1.AnnotationManagedCluster] = explorer.Cluster.String()
	annotations[dplv1.AnnotationLocal] = trueCondition
	annotations[hdplv1alpha1.AnnotationHybridDiscovery] = hdplv1alpha1.HybridDiscoveryEnabled
	dpl.SetAnnotations(annotations)

	return dpl
}

func patchObject(dpl *unstructured.Unstructured, metaobj *unstructured.Unstructured, explorer *utils.Explorer) (*unstructured.Unstructured, error) {
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
	objgvr := explorer.GVKGVRMap[metaobj.GroupVersionKind()]

	ucobj, err := explorer.DynamicMCClient.Resource(objgvr).Namespace(metaobj.GetNamespace()).Get(metaobj.GetName(), metav1.GetOptions{})
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
	ucobj, err = explorer.DynamicMCClient.Resource(objgvr).Namespace(metaobj.GetNamespace()).Update(ucobj, metav1.UpdateOptions{})
	if err == nil {
		klog.V(packageInfoLogLevel).Info("Successfully patched object ", metaobj.GetNamespace()+"/"+metaobj.GetName())
	}
	return ucobj, err
}

func locateDeployableForObject(metaobj metav1.Object, explorer *utils.Explorer) (*dplv1.Deployable, error) {
	gvr := explorer.GVKGVRMap[deployableGVK]

	dpllist, err := explorer.DynamicHubClient.Resource(gvr).Namespace(explorer.Cluster.Namespace).List(metav1.ListOptions{})
	if err != nil {
		klog.Error("Failed to list deployable objects from hub cluster namespace with error:", err)
		return nil, err
	}

	var objdpl *dplv1.Deployable

	for _, dpl := range dpllist.Items {
		annotations := dpl.GetAnnotations()
		if annotations == nil {
			continue
		}

		key := types.NamespacedName{
			Namespace: metaobj.GetNamespace(),
			Name:      metaobj.GetName(),
		}.String()

		srcobj, ok := annotations[corev1alpha1.SourceObject]
		if ok && srcobj == key {
			return objdpl, nil
		}
	}

	return objdpl, nil
}

func locateObjectForDeployable(dpl metav1.Object, explorer *utils.Explorer) (*unstructured.Unstructured, error) {
	uc, err := runtime.DefaultUnstructuredConverter.ToUnstructured(dpl)
	if err != nil {
		klog.Error("Failed to convert object to unstructured with error:", err)
		return nil, err
	}

	kind, found, err := unstructured.NestedString(uc, "spec", "template", "kind")
	if !found || err != nil {
		klog.Error("Cannot get the wrapped object kind for deployable ", dpl.GetNamespace()+"/"+dpl.GetName())
		return nil, err
	}

	gv, found, err := unstructured.NestedString(uc, "spec", "template", "apiVersion")
	if !found || err != nil {
		klog.Error("Cannot get the wrapped object apiversion for deployable ", dpl.GetNamespace()+"/"+dpl.GetName())
		return nil, err
	}

	name, found, err := unstructured.NestedString(uc, "spec", "template", "metadata", "name")
	if !found || err != nil {
		klog.Error("Cannot get the wrapped object name for deployable ", dpl.GetNamespace()+"/"+dpl.GetName())
		return nil, err
	}

	namespace, found, err := unstructured.NestedString(uc, "spec", "template", "metadata", "namespace")
	if !found || err != nil {
		klog.Error("Cannot get the wrapped object namespace for deployable ", dpl.GetNamespace()+"/"+dpl.GetName())
		return nil, err
	}

	gvk := schema.GroupVersionKind{
		Group:   utils.StripVersion(gv),
		Version: utils.StripGroup(gv),
		Kind:    kind,
	}

	gvr := explorer.GVKGVRMap[gvk]

	if _, ok := explorer.GVKGVRMap[gvk]; !ok {
		klog.Error("Cannot get GVR for GVK ", gvk.String()+" for deployable "+dpl.GetNamespace()+"/"+dpl.GetName())
		return nil, err
	}

	obj, err := explorer.DynamicMCClient.Resource(gvr).Namespace(namespace).Get(name, metav1.GetOptions{})
	if obj == nil || err != nil {
		if errors.IsNotFound(err) {
			klog.Error("Cannot find the wrapped object for deployable ", dpl.GetNamespace()+"/"+dpl.GetName())
			return nil, nil
		}

		return nil, err
	}

	return obj, nil
}

var (
	obsoleteAnnotations = []string{
		"kubectl.kubernetes.io/last-applied-configuration",
	}
)

func prepareTemplate(metaobj metav1.Object) {
	var emptyuid types.UID

	metaobj.SetUID(emptyuid)
	metaobj.SetSelfLink("")
	metaobj.SetResourceVersion("")
	metaobj.SetGeneration(0)
	metaobj.SetCreationTimestamp(metav1.Time{})

	annotations := metaobj.GetAnnotations()
	if annotations != nil {
		for _, k := range obsoleteAnnotations {
			delete(annotations, k)
		}

		metaobj.SetAnnotations(annotations)
	}
}
