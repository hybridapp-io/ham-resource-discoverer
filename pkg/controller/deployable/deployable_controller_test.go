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
	"testing"

	. "github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	corev1alpha1 "github.com/hybridapp-io/ham-resource-discoverer/pkg/apis/core/v1alpha1"

	dplv1alpha1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/apps/v1"
)

var (
	mcName = "managedcluster"

	mcServiceName = "mysql-svc"
	mcSTSName     = "wordpress-webserver"

	applicationName  = "wordpress-01"
	appLabelSelector = "app.kubernetes.io/name"

	cluster = types.NamespacedName{
		Name:      mcName,
		Namespace: mcName,
	}

	mcService = &v1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        mcServiceName,
			Namespace:   mcName,
			Annotations: map[string]string{appLabelSelector: applicationName},
			Labels:      map[string]string{appLabelSelector: applicationName},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Port: 3306,
				},
			},
		},
	}

	mcSTS = &apps.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "StatefulSet",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcSTSName,
			Namespace: mcName,
			Labels:    map[string]string{appLabelSelector: applicationName},
		},
		Spec: apps.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{appLabelSelector: applicationName},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{appLabelSelector: applicationName},
				},
			},
		},
	}
	mcSVCDeployable = &dplv1alpha1.Deployable{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcServiceName,
			Namespace: mcName,
			Annotations: map[string]string{
				corev1alpha1.AnnotationDiscovered: "true",
			},
			Labels: map[string]string{
				corev1alpha1.AnnotationDiscovered: "true",
			},
		},
		Spec: dplv1alpha1.DeployableSpec{
			Template: &runtime.RawExtension{
				Object: mcService,
			},
		},
	}

	mcSTSDeployable = &dplv1alpha1.Deployable{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcSTSName,
			Namespace: mcName,
			Annotations: map[string]string{
				corev1alpha1.AnnotationDiscovered: "true",
			},
			Labels: map[string]string{
				corev1alpha1.AnnotationDiscovered: "true",
			},
		},
		Spec: dplv1alpha1.DeployableSpec{
			Template: &runtime.RawExtension{
				Object: mcSTS,
			},
		},
	}
)

func TestNoGroupObject(t *testing.T) {
	g := NewWithT(t)
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	rec, _ := newReconciler(mgr, hubClusterConfig, cluster)
	ds := SetupDeployableSync(rec)

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	ds.start()
	defer ds.stop()

	mcDynamicClient := rec.explorer.DynamicMCClient
	hubDynamicClient := rec.explorer.DynamicHubClient

	// create the local resource
	svc := mcService.DeepCopy()
	svc.Annotations["test_annotation"] = trueCondition
	svc.Labels["test_label"] = trueCondition

	svcGVR := rec.explorer.GVKGVRMap[svc.GroupVersionKind()]
	dplGVR := rec.explorer.GVKGVRMap[deployableGVK]

	svcUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(svc)
	uc := &unstructured.Unstructured{}
	uc.SetUnstructuredContent(svcUC)

	if _, err = mcDynamicClient.Resource(svcGVR).Namespace(svc.Namespace).Create(uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = mcDynamicClient.Resource(svcGVR).Namespace(svc.Namespace).Delete(svc.Name, &metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	// create the deployable on the hub
	dpl := mcSVCDeployable.DeepCopy()
	dplUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(dpl)
	uc = &unstructured.Unstructured{}
	uc.SetUnstructuredContent(dplUC)
	uc.SetGroupVersionKind(deployableGVK)

	if _, err = hubDynamicClient.Resource(dplGVR).Namespace(dpl.Namespace).Create(uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = hubDynamicClient.Resource(dplGVR).Namespace(dpl.Namespace).Delete(dpl.Name, &metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	// wait for the deployable sync to come through on hub
	<-ds.(DeployableSync).createCh
	<-ds.(DeployableSync).updateCh

	// validate the annotation/labels created on the MC object
	newDpl, _ := hubDynamicClient.Resource(dplGVR).Namespace(dpl.Namespace).Get(dpl.Name, metav1.GetOptions{})
	newSVC, _ := mcDynamicClient.Resource(svcGVR).Namespace(svc.Namespace).Get(svc.Name, metav1.GetOptions{})
	g.Expect(newSVC.GetAnnotations()["test_annotation"]).To(Equal(trueCondition))
	g.Expect(newSVC.GetLabels()["test_label"]).To(Equal(trueCondition))
	g.Expect(newDpl.GetLabels()["test_label"]).To(Equal(trueCondition))
}

func TestGroupObject(t *testing.T) {
	g := NewWithT(t)
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	rec, _ := newReconciler(mgr, hubClusterConfig, cluster)
	ds := SetupDeployableSync(rec)

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	ds.start()
	defer ds.stop()

	mcDynamicClient := rec.explorer.DynamicMCClient
	hubDynamicClient := rec.explorer.DynamicHubClient

	// create the local resource
	sts := mcSTS.DeepCopy()

	stsGVR := rec.explorer.GVKGVRMap[sts.GroupVersionKind()]
	dplGVR := rec.explorer.GVKGVRMap[deployableGVK]

	stsUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(sts)
	uc := &unstructured.Unstructured{}
	uc.SetUnstructuredContent(stsUC)

	if _, err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Create(uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Delete(sts.Name, &metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	// create the deployable on the hub
	dpl := mcSTSDeployable.DeepCopy()
	dplUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(dpl)
	uc = &unstructured.Unstructured{}
	uc.SetUnstructuredContent(dplUC)
	uc.SetGroupVersionKind(deployableGVK)

	if _, err = hubDynamicClient.Resource(dplGVR).Namespace(dpl.Namespace).Create(uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = hubDynamicClient.Resource(dplGVR).Namespace(dpl.Namespace).Delete(dpl.Name, &metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	// wait for the deployable sync to come through on hub
	<-ds.(DeployableSync).createCh
	<-ds.(DeployableSync).updateCh

	// update the deployable and add labels and annotations

	newDpl, _ := hubDynamicClient.Resource(dplGVR).Namespace(dpl.Namespace).Get(dpl.Name, metav1.GetOptions{})
	newSTS, _ := mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Get(sts.Name, metav1.GetOptions{})

	// add a new label on the sts and expect to find it on the dpl
	labels, _, _ := unstructured.NestedStringMap(newSTS.Object, "metadata", "labels")
	labels["test_label"] = trueCondition

	if err = unstructured.SetNestedStringMap(newSTS.Object, labels, "metadata", "labels"); err != nil {
		klog.Error(err)
		t.Fail()
	}

	if _, err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Update(newSTS, metav1.UpdateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	annotations, _, _ := unstructured.NestedStringMap(newDpl.Object, "metadata", "annotations")
	annotations["test_annotation"] = trueCondition

	if err = unstructured.SetNestedStringMap(newDpl.Object, annotations, "metadata", "annotations"); err != nil {
		klog.Error(err)
		t.Fail()
	}

	// update dpl label to trigger reconcile
	if _, err = hubDynamicClient.Resource(dplGVR).Namespace(dpl.Namespace).Update(newDpl, metav1.UpdateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	<-ds.(DeployableSync).updateCh

	updatedDpl, _ := hubDynamicClient.Resource(dplGVR).Namespace(dpl.Namespace).Get(dpl.Name, metav1.GetOptions{})

	// validate the annotation/labels created on the MC object
	g.Expect(updatedDpl.GetAnnotations()["test_annotation"]).To(Equal(trueCondition))
	g.Expect(updatedDpl.GetLabels()["test_label"]).To(Equal(trueCondition))

	// turn off the hybrid discovered flag and make sure reconciliation does not happen (test_new_label will not be added to dpl)
	updatedSTS, _ := mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Get(sts.Name, metav1.GetOptions{})
	updatedSTS.GetLabels()["test_new_label"] = trueCondition

	if _, err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Update(updatedSTS, metav1.UpdateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	annotations, _, _ = unstructured.NestedStringMap(updatedDpl.Object, "metadata", "annotations")
	delete(annotations, corev1alpha1.AnnotationDiscovered)

	if err = unstructured.SetNestedStringMap(updatedDpl.Object, annotations, "metadata", "annotations"); err != nil {
		klog.Error(err)
		t.Fail()
	}

	if _, err = hubDynamicClient.Resource(dplGVR).Namespace(dpl.Namespace).Update(updatedDpl, metav1.UpdateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	<-ds.(DeployableSync).updateCh

	updatedDpl, _ = hubDynamicClient.Resource(dplGVR).Namespace(dpl.Namespace).Get(dpl.Name, metav1.GetOptions{})
	g.Expect(updatedDpl.GetLabels()).To(Not(ContainElement("test_new_label")))
}

func TestDeployableCleanup(t *testing.T) {
	g := NewWithT(t)
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	rec, _ := newReconciler(mgr, hubClusterConfig, cluster)
	ds := SetupDeployableSync(rec)

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	ds.start()
	defer ds.stop()

	mcDynamicClient := rec.explorer.DynamicMCClient
	hubDynamicClient := rec.explorer.DynamicHubClient

	// create the local resource
	sts := mcSTS.DeepCopy()

	stsGVR := rec.explorer.GVKGVRMap[sts.GroupVersionKind()]
	dplGVR := rec.explorer.GVKGVRMap[deployableGVK]

	stsUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(sts)
	uc := &unstructured.Unstructured{}
	uc.SetUnstructuredContent(stsUC)

	if _, err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Create(uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	// create the deployable on the hub
	dplObj := mcSTSDeployable.DeepCopy()
	dplObj.GetAnnotations()["test_annotation"] = trueCondition
	dplUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(dplObj)
	uc = &unstructured.Unstructured{}
	uc.SetUnstructuredContent(dplUC)
	uc.SetGroupVersionKind(deployableGVK)

	if _, err = hubDynamicClient.Resource(dplGVR).Namespace(dplObj.Namespace).Create(uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	// wait for the deployable sync to come through on hub
	<-ds.(DeployableSync).createCh
	<-ds.(DeployableSync).updateCh

	// delete the sts
	if err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Delete(sts.Name, &metav1.DeleteOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	// trigger reconciliation on deployable and make sure it gets cleaned up
	dpl, _ := hubDynamicClient.Resource(dplGVR).Namespace(dplObj.Namespace).Get(dplObj.Name, metav1.GetOptions{})
	annotations, _, _ := unstructured.NestedStringMap(dpl.Object, "metadata", "annotations")
	delete(annotations, "test_annotation")

	if err = unstructured.SetNestedStringMap(dpl.Object, annotations, "metadata", "annotations"); err != nil {
		klog.Error(err)
		t.Fail()
	}

	if _, err = hubDynamicClient.Resource(dplGVR).Namespace(dplObj.Namespace).Update(dpl, metav1.UpdateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	<-ds.(DeployableSync).updateCh

	dpl, _ = hubDynamicClient.Resource(dplGVR).Namespace(dplObj.Namespace).Get(dplObj.Name, metav1.GetOptions{})

	g.Expect(dpl).To(BeNil())
}
