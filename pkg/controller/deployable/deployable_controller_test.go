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

	hdplv1alpha1 "github.com/hybridapp-io/ham-deployable-operator/pkg/apis/core/v1alpha1"
	"github.com/hybridapp-io/ham-resource-discoverer/pkg/synchronizer/ocm"
	"github.com/hybridapp-io/ham-resource-discoverer/pkg/utils"
	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/apps/v1"
	subv1 "github.com/open-cluster-management/multicloud-operators-subscription/pkg/apis/apps/v1"
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
)

func TestNoGroupObject(t *testing.T) {
	g := NewWithT(t)
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	explorer, err := utils.InitExplorer(hubClusterConfig, mgr.GetConfig(), cluster)
	hubSynchronizer := &ocm.HubSynchronizer{}

	rec, _ := NewReconciler(mgr, hubClusterConfig, cluster, explorer, hubSynchronizer)

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
	svc := mcService.DeepCopy()

	svcGVR := explorer.GVKGVRMap[svc.GroupVersionKind()]

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
	deployablegvr := explorer.GVKGVRMap[deployableGVK]
	if _, err := hubDynamicClient.Resource(deployablegvr).Namespace(dpl.Namespace).Create(uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = hubDynamicClient.Resource(deployableGVR).Namespace(dpl.Namespace).Delete(dpl.Name, &metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	if _, err := hubDynamicClient.Resource(deployableGVR).Namespace(dpl.Namespace).Get(mcSVCDeployable.Name, metav1.GetOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}
	// wait for the deployable sync to come through on hub
	<-ds.(DeployableSync).createCh
	<-ds.(DeployableSync).updateCh

	// validate the annotation/labels created on the MC object
	newSVC, _ := mcDynamicClient.Resource(svcGVR).Namespace(svc.Namespace).Get(svc.Name, metav1.GetOptions{})
	annotations := newSVC.GetAnnotations()
	g.Expect(annotations[dplv1.AnnotationHosting]).To(Equal(dpl.Namespace + "/" + dpl.Name))
	g.Expect(annotations[subv1.AnnotationHosting]).To(Equal("/"))
	g.Expect(annotations[subv1.AnnotationSyncSource]).To(Equal("subnsdpl-/"))
}

func TestRefreshObjectWithDiscovery(t *testing.T) {
	g := NewWithT(t)
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	explorer, err := utils.InitExplorer(hubClusterConfig, mgr.GetConfig(), cluster)
	hubSynchronizer := &ocm.HubSynchronizer{}

	rec, _ := NewReconciler(mgr, hubClusterConfig, cluster, explorer, hubSynchronizer)

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

	if _, err = hubDynamicClient.Resource(deployableGVR).Namespace(dpl.Namespace).Create(uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = hubDynamicClient.Resource(deployableGVR).Namespace(dpl.Namespace).Delete(dpl.Name, &metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	// wait for the deployable sync to come through on hub
	<-ds.(DeployableSync).createCh
	<-ds.(DeployableSync).updateCh

	// remove the object subscription anno
	newDpl, _ := hubDynamicClient.Resource(deployableGVR).Namespace(dpl.Namespace).Get(dpl.Name, metav1.GetOptions{})
	newSTS, _ := mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Get(sts.Name, metav1.GetOptions{})

	stsAnnotations := newSTS.GetAnnotations()
	delete(stsAnnotations, dplv1.AnnotationHosting)
	delete(stsAnnotations, subv1.AnnotationHosting)
	delete(stsAnnotations, subv1.AnnotationSyncSource)
	newSTS.SetAnnotations(stsAnnotations)
	if _, err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Update(newSTS, metav1.UpdateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	dplAnnotations := newDpl.GetAnnotations()
	dplAnnotations[hdplv1alpha1.AnnotationHybridDiscovery] = hdplv1alpha1.HybridDiscoveryEnabled
	newDpl.SetAnnotations(dplAnnotations)

	if _, err = hubDynamicClient.Resource(deployableGVR).Namespace(dpl.Namespace).Update(newDpl, metav1.UpdateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}
	<-ds.(DeployableSync).updateCh

	updatedSTS, _ := mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Get(sts.Name, metav1.GetOptions{})
	annotations := updatedSTS.GetAnnotations()
	g.Expect(annotations[dplv1.AnnotationHosting]).To(Equal(dpl.Namespace + "/" + dpl.Name))
	g.Expect(annotations[subv1.AnnotationHosting]).To(Equal("/"))
	g.Expect(annotations[subv1.AnnotationSyncSource]).To(Equal("subnsdpl-/"))
}

func TestRefreshObjectWithoutDiscovery(t *testing.T) {
	g := NewWithT(t)
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	explorer, err := utils.InitExplorer(hubClusterConfig, mgr.GetConfig(), cluster)
	hubSynchronizer := &ocm.HubSynchronizer{}

	rec, _ := NewReconciler(mgr, hubClusterConfig, cluster, explorer, hubSynchronizer)

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

	if _, err = hubDynamicClient.Resource(deployableGVR).Namespace(dpl.Namespace).Create(uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = hubDynamicClient.Resource(deployableGVR).Namespace(dpl.Namespace).Delete(dpl.Name, &metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	// wait for the deployable sync to come through on hub
	<-ds.(DeployableSync).createCh
	<-ds.(DeployableSync).updateCh

	// remove the object subscription anno
	newDpl, _ := hubDynamicClient.Resource(deployableGVR).Namespace(dpl.Namespace).Get(dpl.Name, metav1.GetOptions{})
	newSTS, _ := mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Get(sts.Name, metav1.GetOptions{})

	stsAnnotations := newSTS.GetAnnotations()
	delete(stsAnnotations, dplv1.AnnotationHosting)
	delete(stsAnnotations, subv1.AnnotationHosting)
	delete(stsAnnotations, subv1.AnnotationSyncSource)
	newSTS.SetAnnotations(stsAnnotations)
	if _, err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Update(newSTS, metav1.UpdateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	dplAnnotations := newDpl.GetAnnotations()
	newDpl.SetAnnotations(dplAnnotations)

	if _, err = hubDynamicClient.Resource(deployableGVR).Namespace(dpl.Namespace).Update(newDpl, metav1.UpdateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	// discovery flag has been turned into completed from enabled, so no changes on the resource expected
	updatedSTS, _ := mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Get(sts.Name, metav1.GetOptions{})
	annotations := updatedSTS.GetAnnotations()
	g.Expect(annotations[dplv1.AnnotationHosting]).To(BeEmpty())
	g.Expect(annotations[subv1.AnnotationHosting]).To(BeEmpty())
	g.Expect(annotations[subv1.AnnotationSyncSource]).To(BeEmpty())
}

func TestRefreshOwnershipChange(t *testing.T) {
	g := NewWithT(t)
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	explorer, err := utils.InitExplorer(hubClusterConfig, mgr.GetConfig(), cluster)
	hubSynchronizer := &ocm.HubSynchronizer{}

	rec, _ := NewReconciler(mgr, hubClusterConfig, cluster, explorer, hubSynchronizer)

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
	dpl1 := mcSTSDeployable.DeepCopy()
	dpl1UC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(dpl1)
	uc1 := &unstructured.Unstructured{}
	uc1.SetUnstructuredContent(dpl1UC)
	uc1.SetGroupVersionKind(deployableGVK)

	if _, err = hubDynamicClient.Resource(deployableGVR).Namespace(dpl1.Namespace).Create(uc1, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = hubDynamicClient.Resource(deployableGVR).Namespace(dpl1.Namespace).Delete(dpl1.Name, &metav1.DeleteOptions{}); err != nil {
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

	if _, err = hubDynamicClient.Resource(deployableGVR).Namespace(dpl2.Namespace).Create(uc2, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = hubDynamicClient.Resource(deployableGVR).Namespace(dpl2.Namespace).Delete(dpl2.Name, &metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	// wait for the deployable sync to come through on hub
	<-ds.(DeployableSync).createCh

	updatedSTS, _ := mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Get(sts.Name, metav1.GetOptions{})
	annotations := updatedSTS.GetAnnotations()
	g.Expect(annotations[dplv1.AnnotationHosting]).To(Equal(dpl1.Namespace + "/" + dpl1.Name))
	g.Expect(annotations[subv1.AnnotationHosting]).To(Equal("/"))
	g.Expect(annotations[subv1.AnnotationSyncSource]).To(Equal("subnsdpl-/"))
}

func TestDeployableCleanup(t *testing.T) {
	g := NewWithT(t)
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	explorer, err := utils.InitExplorer(hubClusterConfig, mgr.GetConfig(), cluster)
	if err != nil {
		klog.Error("Failed to initialize the explorer")
		t.Fail()
	}

	hubSynchronizer := &ocm.HubSynchronizer{}

	rec, _ := NewReconciler(mgr, hubClusterConfig, cluster, explorer, hubSynchronizer)

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

	if _, err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Create(uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	// create the deployable on the hub
	dplObj := mcSTSDeployable.DeepCopy()
	dplUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(dplObj)
	uc = &unstructured.Unstructured{}
	uc.SetUnstructuredContent(dplUC)
	uc.SetGroupVersionKind(deployableGVK)

	if _, err = hubDynamicClient.Resource(deployableGVR).Namespace(dplObj.Namespace).Create(uc, metav1.CreateOptions{}); err != nil {
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
	newDpl, _ := hubDynamicClient.Resource(deployableGVR).Namespace(dplObj.Namespace).Get(dplObj.Name, metav1.GetOptions{})
	dplAnnotations := newDpl.GetAnnotations()
	dplAnnotations[hdplv1alpha1.AnnotationHybridDiscovery] = hdplv1alpha1.HybridDiscoveryEnabled
	newDpl.SetAnnotations(dplAnnotations)

	if _, err = hubDynamicClient.Resource(deployableGVR).Namespace(newDpl.GetNamespace()).Update(newDpl, metav1.UpdateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}
	<-ds.(DeployableSync).updateCh
	dpl, _ := hubDynamicClient.Resource(deployableGVR).Namespace(dplObj.Namespace).Get(dplObj.Name, metav1.GetOptions{})

	g.Expect(dpl).To(BeNil())
}
