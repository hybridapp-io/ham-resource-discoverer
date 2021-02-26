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
	"context"
	"testing"

	. "github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	hdplv1alpha1 "github.com/hybridapp-io/ham-deployable-operator/pkg/apis/core/v1alpha1"
	"github.com/hybridapp-io/ham-resource-discoverer/pkg/utils"
	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/apps/v1"
	subv1 "github.com/open-cluster-management/multicloud-operators-subscription/pkg/apis/apps/v1"
)

var (
	mcName = "managedcluster"
	mcNS   = v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: mcName,
		},
	}

	mcServiceName = "mysql-svc"
	mcSTSName     = "wordpress-webserver"
	mcPodName     = "wordpress-pod"

	applicationName  = "wordpress-01"
	appLabelSelector = "app.kubernetes.io/name"

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
	mcSVCDeployable = &dplv1.Deployable{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcServiceName,
			Namespace: mcName,
			Annotations: map[string]string{
				hdplv1alpha1.AnnotationHybridDiscovery: hdplv1alpha1.HybridDiscoveryEnabled,
			},
			Labels: map[string]string{
				hdplv1alpha1.AnnotationHybridDiscovery: hdplv1alpha1.HybridDiscoveryEnabled,
			},
		},
		Spec: dplv1.DeployableSpec{
			Template: &runtime.RawExtension{
				Object: mcService,
			},
		},
	}

	mcPodDeployable = &dplv1.Deployable{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcPodName,
			Namespace: mcName,
			Annotations: map[string]string{
				hdplv1alpha1.AnnotationHybridDiscovery: hdplv1alpha1.HybridDiscoveryEnabled,
			},
			Labels: map[string]string{
				hdplv1alpha1.AnnotationHybridDiscovery: hdplv1alpha1.HybridDiscoveryEnabled,
			},
		},
		Spec: dplv1.DeployableSpec{
			Template: &runtime.RawExtension{
				Object: mcPod,
			},
		},
	}

	mcSTSDeployable = &dplv1.Deployable{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcSTSName,
			Namespace: mcName,
			Annotations: map[string]string{
				hdplv1alpha1.AnnotationHybridDiscovery: hdplv1alpha1.HybridDiscoveryEnabled,
			},
			Labels: map[string]string{
				hdplv1alpha1.AnnotationHybridDiscovery: hdplv1alpha1.HybridDiscoveryEnabled,
			},
		},
		Spec: dplv1.DeployableSpec{
			Template: &runtime.RawExtension{
				Object: mcSTS,
			},
		},
	}

	mcPod = &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcPodName,
			Namespace: mcName,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "Service",
					Name:       mcServiceName,
					APIVersion: "v1",
					UID:        "1",
				},
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "container",
					Image: "image",
				},
			},
		},
	}
)

func TestNoGroupObject(t *testing.T) {
	g := NewWithT(t)
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	explorer, err := utils.InitExplorer(hubClusterConfig, mgr.GetConfig(), mcName)

	c := mgr.GetClient()
	rec, _ := NewReconciler(mgr, hubClusterConfig, mcName, explorer)

	ds := SetupDeployableSync(rec)

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	ds.Start()
	defer ds.Stop()

	mcDynamicClient := explorer.DynamicMCClient
	hubDynamicClient := explorer.DynamicHubClient

	nsMC := mcNS.DeepCopy()
	g.Expect(c.Create(context.TODO(), nsMC)).To(Succeed())

	nsHub := mcNS.DeepCopy()
	nsGVR := schema.GroupVersionResource{
		Version:  "v1",
		Resource: "namespaces",
	}
	ucUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(nsHub)
	ucNS := &unstructured.Unstructured{}
	ucNS.SetUnstructuredContent(ucUC)
	_, err = hubDynamicClient.Resource(nsGVR).Create(context.TODO(), ucNS, metav1.CreateOptions{})
	g.Expect(err).ShouldNot(HaveOccurred())

	// create the local resource
	svc := mcService.DeepCopy()

	svcGVR := explorer.GVKGVRMap[svc.GroupVersionKind()]

	svcUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(svc)
	uc := &unstructured.Unstructured{}
	uc.SetUnstructuredContent(svcUC)

	if _, err = mcDynamicClient.Resource(svcGVR).Namespace(svc.Namespace).Create(context.TODO(), uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = mcDynamicClient.Resource(svcGVR).Namespace(svc.Namespace).Delete(context.TODO(), svc.Name, metav1.DeleteOptions{}); err != nil {
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
	deployablegvr := explorer.GVKGVRMap[deployableGVK]
	if _, err := hubDynamicClient.Resource(deployablegvr).Namespace(dpl.Namespace).Create(context.TODO(), uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = hubDynamicClient.Resource(deployableGVR).Namespace(dpl.Namespace).Delete(context.TODO(), dpl.Name, metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	if _, err := hubDynamicClient.Resource(deployableGVR).Namespace(dpl.Namespace).Get(context.TODO(), mcSVCDeployable.Name, metav1.GetOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}
	// wait for the deployable sync to come through on hub
	<-ds.(DeployableSync).createCh
	<-ds.(DeployableSync).updateCh

	// validate the annotation/labels created on the MC object
	newSVC, _ := mcDynamicClient.Resource(svcGVR).Namespace(svc.Namespace).Get(context.TODO(), svc.Name, metav1.GetOptions{})
	annotations := newSVC.GetAnnotations()
	g.Expect(annotations[dplv1.AnnotationHosting]).To(Equal(dpl.Namespace + "/" + dpl.Name))
	g.Expect(annotations[subv1.AnnotationHosting]).To(Equal("/"))
	g.Expect(annotations[subv1.AnnotationSyncSource]).To(Equal("subnsdpl-/"))
}

func TestObjectWithOwnerReference(t *testing.T) {
	g := NewWithT(t)
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	explorer, err := utils.InitExplorer(hubClusterConfig, mgr.GetConfig(), mcName)

	rec, _ := NewReconciler(mgr, hubClusterConfig, mcName, explorer)

	ds := SetupDeployableSync(rec)

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	ds.Start()
	defer ds.Stop()

	mcDynamicClient := explorer.DynamicMCClient
	hubDynamicClient := explorer.DynamicHubClient

	// create the local resources
	svc := mcService.DeepCopy()

	svcGVR := explorer.GVKGVRMap[svc.GroupVersionKind()]

	svcUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(svc)
	uc := &unstructured.Unstructured{}
	uc.SetUnstructuredContent(svcUC)

	if _, err = mcDynamicClient.Resource(svcGVR).Namespace(svc.Namespace).Create(context.TODO(), uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = mcDynamicClient.Resource(svcGVR).Namespace(svc.Namespace).Delete(context.TODO(), svc.Name, metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	pod := mcPod.DeepCopy()

	podGVR := explorer.GVKGVRMap[pod.GroupVersionKind()]

	podUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(pod)
	ucPod := &unstructured.Unstructured{}
	ucPod.SetUnstructuredContent(podUC)

	if _, err = mcDynamicClient.Resource(podGVR).Namespace(pod.Namespace).Create(context.TODO(), ucPod, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = mcDynamicClient.Resource(podGVR).Namespace(pod.Namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	// create the deployable on the hub
	dpl := mcPodDeployable.DeepCopy()
	dplUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(dpl)
	uc = &unstructured.Unstructured{}
	uc.SetUnstructuredContent(dplUC)
	uc.SetGroupVersionKind(deployableGVK)
	deployablegvr := explorer.GVKGVRMap[deployableGVK]
	if _, err := hubDynamicClient.Resource(deployablegvr).Namespace(dpl.Namespace).Create(context.TODO(), uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = hubDynamicClient.Resource(deployableGVR).Namespace(dpl.Namespace).Delete(context.TODO(), dpl.Name, metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	if _, err := hubDynamicClient.Resource(deployableGVR).Namespace(dpl.Namespace).Get(context.TODO(), mcPodDeployable.Name, metav1.GetOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}
	// wait for the deployable sync to come through on hub
	<-ds.(DeployableSync).createCh
	<-ds.(DeployableSync).updateCh

	hubDep, err := hubDynamicClient.Resource(deployableGVR).Namespace(dpl.Namespace).Get(context.TODO(), mcPodDeployable.Name, metav1.GetOptions{})

	g.Expect(hubDep.GetKind()).To(Equal("Deployable"))

	// Check that the template points to the service, not a pod

	ucdep, err := runtime.DefaultUnstructuredConverter.ToUnstructured(hubDep)
	kind, found, err := unstructured.NestedString(ucdep, "spec", "template", "kind")
	if !found || err != nil {
		klog.Error("Cannot get the wrapped object kind for deployable ", hubDep.GetNamespace()+"/"+hubDep.GetName())
		t.Fail()
	}

	name, found, err := unstructured.NestedString(ucdep, "spec", "template", "metadata", "name")
	if !found || err != nil {
		klog.Error("Cannot get the wrapped object name for deployable ", hubDep.GetNamespace()+"/"+hubDep.GetName())
		t.Fail()
	}

	namespace, found, err := unstructured.NestedString(ucdep, "spec", "template", "metadata", "namespace")
	if !found || err != nil {
		klog.Error("Cannot get the wrapped object namespace for deployable ", hubDep.GetNamespace()+"/"+hubDep.GetName())
		t.Fail()
	}

	g.Expect(kind).To(Equal("Service"))
	g.Expect(name).To(Equal(mcServiceName))
	g.Expect(namespace).To(Equal(mcName))
}

func TestRefreshObjectWithDiscovery(t *testing.T) {
	g := NewWithT(t)
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	explorer, err := utils.InitExplorer(hubClusterConfig, mgr.GetConfig(), mcName)

	rec, _ := NewReconciler(mgr, hubClusterConfig, mcName, explorer)

	ds := SetupDeployableSync(rec)

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	ds.Start()
	defer ds.Stop()

	mcDynamicClient := explorer.DynamicMCClient
	hubDynamicClient := explorer.DynamicHubClient

	// create the local resource
	sts := mcSTS.DeepCopy()

	stsGVR := explorer.GVKGVRMap[sts.GroupVersionKind()]

	stsUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(sts)
	uc := &unstructured.Unstructured{}
	uc.SetUnstructuredContent(stsUC)

	if _, err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Create(context.TODO(), uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Delete(context.TODO(), sts.Name, metav1.DeleteOptions{}); err != nil {
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

	if _, err = hubDynamicClient.Resource(deployableGVR).Namespace(dpl.Namespace).Create(context.TODO(), uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = hubDynamicClient.Resource(deployableGVR).Namespace(dpl.Namespace).Delete(context.TODO(), dpl.Name, metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	// wait for the deployable sync to come through on hub
	<-ds.(DeployableSync).createCh
	<-ds.(DeployableSync).updateCh

	// remove the object subscription anno
	newDpl, _ := hubDynamicClient.Resource(deployableGVR).Namespace(dpl.Namespace).Get(context.TODO(), dpl.Name, metav1.GetOptions{})
	newSTS, _ := mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})

	stsAnnotations := newSTS.GetAnnotations()
	delete(stsAnnotations, dplv1.AnnotationHosting)
	delete(stsAnnotations, subv1.AnnotationHosting)
	delete(stsAnnotations, subv1.AnnotationSyncSource)
	newSTS.SetAnnotations(stsAnnotations)
	if _, err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Update(context.TODO(), newSTS, metav1.UpdateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	dplAnnotations := newDpl.GetAnnotations()
	dplAnnotations[hdplv1alpha1.AnnotationHybridDiscovery] = hdplv1alpha1.HybridDiscoveryEnabled
	newDpl.SetAnnotations(dplAnnotations)

	if _, err = hubDynamicClient.Resource(deployableGVR).Namespace(dpl.Namespace).Update(context.TODO(), newDpl, metav1.UpdateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}
	<-ds.(DeployableSync).updateCh

	updatedSTS, _ := mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
	annotations := updatedSTS.GetAnnotations()
	g.Expect(annotations[dplv1.AnnotationHosting]).To(Equal(dpl.Namespace + "/" + dpl.Name))
	g.Expect(annotations[subv1.AnnotationHosting]).To(Equal("/"))
	g.Expect(annotations[subv1.AnnotationSyncSource]).To(Equal("subnsdpl-/"))
}

func TestRefreshObjectWithoutDiscovery(t *testing.T) {
	g := NewWithT(t)
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	explorer, err := utils.InitExplorer(hubClusterConfig, mgr.GetConfig(), mcName)

	rec, _ := NewReconciler(mgr, hubClusterConfig, mcName, explorer)

	ds := SetupDeployableSync(rec)

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	ds.Start()
	defer ds.Stop()

	mcDynamicClient := explorer.DynamicMCClient
	hubDynamicClient := explorer.DynamicHubClient

	// create the local resource
	sts := mcSTS.DeepCopy()

	stsGVR := explorer.GVKGVRMap[sts.GroupVersionKind()]

	stsUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(sts)
	uc := &unstructured.Unstructured{}
	uc.SetUnstructuredContent(stsUC)

	if _, err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Create(context.TODO(), uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Delete(context.TODO(), sts.Name, metav1.DeleteOptions{}); err != nil {
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

	if _, err = hubDynamicClient.Resource(deployableGVR).Namespace(dpl.Namespace).Create(context.TODO(), uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = hubDynamicClient.Resource(deployableGVR).Namespace(dpl.Namespace).Delete(context.TODO(), dpl.Name, metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	// wait for the deployable sync to come through on hub
	<-ds.(DeployableSync).createCh
	<-ds.(DeployableSync).updateCh

	// remove the object subscription anno
	newDpl, _ := hubDynamicClient.Resource(deployableGVR).Namespace(dpl.Namespace).Get(context.TODO(), dpl.Name, metav1.GetOptions{})
	newSTS, _ := mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})

	stsAnnotations := newSTS.GetAnnotations()
	delete(stsAnnotations, dplv1.AnnotationHosting)
	delete(stsAnnotations, subv1.AnnotationHosting)
	delete(stsAnnotations, subv1.AnnotationSyncSource)
	newSTS.SetAnnotations(stsAnnotations)
	if _, err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Update(context.TODO(), newSTS, metav1.UpdateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	dplAnnotations := newDpl.GetAnnotations()
	newDpl.SetAnnotations(dplAnnotations)

	if _, err = hubDynamicClient.Resource(deployableGVR).Namespace(dpl.Namespace).Update(context.TODO(), newDpl, metav1.UpdateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	// discovery flag has been turned into completed from enabled, so no changes on the resource expected
	updatedSTS, _ := mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
	annotations := updatedSTS.GetAnnotations()
	g.Expect(annotations[dplv1.AnnotationHosting]).To(BeEmpty())
	g.Expect(annotations[subv1.AnnotationHosting]).To(BeEmpty())
	g.Expect(annotations[subv1.AnnotationSyncSource]).To(BeEmpty())
}

func TestRefreshOwnershipChange(t *testing.T) {
	g := NewWithT(t)
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	explorer, err := utils.InitExplorer(hubClusterConfig, mgr.GetConfig(), mcName)

	rec, _ := NewReconciler(mgr, hubClusterConfig, mcName, explorer)

	ds := SetupDeployableSync(rec)

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	ds.Start()
	defer ds.Stop()

	mcDynamicClient := explorer.DynamicMCClient
	hubDynamicClient := explorer.DynamicHubClient

	// create the local resource
	sts := mcSTS.DeepCopy()

	stsGVR := explorer.GVKGVRMap[sts.GroupVersionKind()]

	stsUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(sts)
	uc := &unstructured.Unstructured{}
	uc.SetUnstructuredContent(stsUC)

	if _, err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Create(context.TODO(), uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Delete(context.TODO(), sts.Name, metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	// create the deployable on the hub
	dpl1 := mcSTSDeployable.DeepCopy()
	dpl1UC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(dpl1)
	uc1 := &unstructured.Unstructured{}
	uc1.SetUnstructuredContent(dpl1UC)
	uc1.SetGroupVersionKind(deployableGVK)

	if _, err = hubDynamicClient.Resource(deployableGVR).Namespace(dpl1.Namespace).Create(context.TODO(), uc1, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = hubDynamicClient.Resource(deployableGVR).Namespace(dpl1.Namespace).Delete(context.TODO(), dpl1.Name, metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	// wait for the deployable sync to come through on hub
	<-ds.(DeployableSync).createCh
	<-ds.(DeployableSync).updateCh

	// create a new deployable pointing to the same resource
	dpl2 := mcSTSDeployable.DeepCopy()
	dpl2.SetName(mcSTSName + "2")
	dpl2UC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(dpl2)
	uc2 := &unstructured.Unstructured{}
	uc2.SetUnstructuredContent(dpl2UC)
	uc2.SetGroupVersionKind(deployableGVK)

	if _, err = hubDynamicClient.Resource(deployableGVR).Namespace(dpl2.Namespace).Create(context.TODO(), uc2, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = hubDynamicClient.Resource(deployableGVR).Namespace(dpl2.Namespace).Delete(context.TODO(), dpl2.Name, metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	// wait for the deployable sync to come through on hub
	<-ds.(DeployableSync).createCh

	updatedSTS, _ := mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
	annotations := updatedSTS.GetAnnotations()
	g.Expect(annotations[dplv1.AnnotationHosting]).To(Equal(dpl1.Namespace + "/" + dpl1.Name))
	g.Expect(annotations[subv1.AnnotationHosting]).To(Equal("/"))
	g.Expect(annotations[subv1.AnnotationSyncSource]).To(Equal("subnsdpl-/"))
}

func TestSyncDeployable(t *testing.T) {
	g := NewWithT(t)
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	explorer, err := utils.InitExplorer(hubClusterConfig, mgr.GetConfig(), mcName)
	if err != nil {
		klog.Error("Failed to initialize the explorer")
		t.Fail()
	}

	rec, _ := NewReconciler(mgr, hubClusterConfig, mcName, explorer)

	ds := SetupDeployableSync(rec)

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	ds.Start()
	defer ds.Stop()

	mcDynamicClient := explorer.DynamicMCClient
	hubDynamicClient := explorer.DynamicHubClient

	// create the local resource
	sts := mcSTS.DeepCopy()
	stsGVR := explorer.GVKGVRMap[sts.GroupVersionKind()]
	sts.Annotations = make(map[string]string)
	sts.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "to_be_removed"

	stsUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(sts)
	uc := &unstructured.Unstructured{}
	uc.SetUnstructuredContent(stsUC)

	if _, err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Create(context.TODO(), uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}
	defer func() {
		if err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Delete(context.TODO(), sts.Name, metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	err = SyncDeployable(uc, explorer)
	g.Expect(err).ShouldNot(HaveOccurred())

	dplList, _ := hubDynamicClient.Resource(deployableGVR).Namespace(mcName).List(context.TODO(), metav1.ListOptions{})
	g.Expect(dplList.Items).To(HaveLen(1))

	dpl, _ := locateDeployableForObject(uc, explorer)
	g.Expect(dpl).NotTo(BeNil())

	err = SyncDeployable(uc, explorer)
	g.Expect(err).ShouldNot(HaveOccurred())

	// sync again and expect existing deployable to be ignored
	uc, err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Get(context.TODO(), uc.GetName(), metav1.GetOptions{})
	if err != nil {
		klog.Error(err)
		t.Fail()
	}

	err = SyncDeployable(uc, explorer)
	g.Expect(err).ShouldNot(HaveOccurred())

	dplList, _ = hubDynamicClient.Resource(deployableGVR).Namespace(mcName).List(context.TODO(), metav1.ListOptions{})
	g.Expect(dplList.Items).To(HaveLen(1))

	if err = hubDynamicClient.Resource(deployableGVR).Namespace(dpl.Namespace).Delete(context.TODO(), dpl.GetName(), metav1.DeleteOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}
	<-ds.(DeployableSync).deleteCh
}

func TestDeployableCleanup(t *testing.T) {
	g := NewWithT(t)
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	explorer, err := utils.InitExplorer(hubClusterConfig, mgr.GetConfig(), mcName)
	if err != nil {
		klog.Error("Failed to initialize the explorer")
		t.Fail()
	}

	rec, _ := NewReconciler(mgr, hubClusterConfig, mcName, explorer)

	ds := SetupDeployableSync(rec)

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	ds.Start()
	defer ds.Stop()

	mcDynamicClient := explorer.DynamicMCClient
	hubDynamicClient := explorer.DynamicHubClient

	// create the local resource
	sts := mcSTS.DeepCopy()

	stsGVR := explorer.GVKGVRMap[sts.GroupVersionKind()]

	stsUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(sts)
	uc := &unstructured.Unstructured{}
	uc.SetUnstructuredContent(stsUC)

	if _, err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Create(context.TODO(), uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	// create the deployable on the hub
	dplObj := mcSTSDeployable.DeepCopy()
	dplUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(dplObj)
	uc = &unstructured.Unstructured{}
	uc.SetUnstructuredContent(dplUC)
	uc.SetGroupVersionKind(deployableGVK)

	if _, err = hubDynamicClient.Resource(deployableGVR).Namespace(dplObj.Namespace).Create(context.TODO(), uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	// wait for the deployable sync to come through on hub
	<-ds.(DeployableSync).createCh
	<-ds.(DeployableSync).updateCh

	// delete the sts
	if err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Delete(context.TODO(), sts.Name, metav1.DeleteOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	// trigger reconciliation on deployable and make sure it gets cleaned up
	newDpl, _ := hubDynamicClient.Resource(deployableGVR).Namespace(dplObj.Namespace).Get(context.TODO(), dplObj.Name, metav1.GetOptions{})
	dplAnnotations := newDpl.GetAnnotations()
	dplAnnotations[hdplv1alpha1.AnnotationHybridDiscovery] = hdplv1alpha1.HybridDiscoveryEnabled
	newDpl.SetAnnotations(dplAnnotations)

	if _, err = hubDynamicClient.Resource(deployableGVR).Namespace(newDpl.GetNamespace()).Update(context.TODO(), newDpl, metav1.UpdateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}
	<-ds.(DeployableSync).updateCh
	dpl, _ := hubDynamicClient.Resource(deployableGVR).Namespace(dplObj.Namespace).Get(context.TODO(), dplObj.Name, metav1.GetOptions{})

	g.Expect(dpl).To(BeNil())
}

func TestGenericControllerReconcile(t *testing.T) {
	g := NewWithT(t)
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	explorer, err := utils.InitExplorer(hubClusterConfig, mgr.GetConfig(), mcName)
	if err != nil {
		klog.Error(err)
		t.Fail()
	}

	rec, _ := NewReconciler(mgr, hubClusterConfig, mcName, explorer)

	hubDynamicClient := explorer.DynamicHubClient

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	rec.Start()
	dplObj := mcSTSDeployable.DeepCopy()
	dplUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(dplObj)
	uc := &unstructured.Unstructured{}
	uc.SetUnstructuredContent(dplUC)
	uc.SetGroupVersionKind(deployableGVK)

	if _, err = hubDynamicClient.Resource(deployableGVR).Namespace(dplObj.Namespace).Create(context.TODO(), uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer rec.Stop()
}
