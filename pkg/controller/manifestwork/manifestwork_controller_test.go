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

package manifestwork

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
	corev1alpha1 "github.com/hybridapp-io/ham-resource-discoverer/pkg/apis/core/v1alpha1"
	"github.com/hybridapp-io/ham-resource-discoverer/pkg/utils"

	workapiv1 "github.com/open-cluster-management/api/work/v1"
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
	mcSVCManifestWork = &workapiv1.ManifestWork{
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
		Spec: workapiv1.ManifestWorkSpec{
			Workload: workapiv1.ManifestsTemplate{
				Manifests: []workapiv1.Manifest{
					{
						runtime.RawExtension{
							Object: mcService,
						},
					},
				},
			},
		},
	}

	mcPodManifestWork = &workapiv1.ManifestWork{
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
		Spec: workapiv1.ManifestWorkSpec{
			Workload: workapiv1.ManifestsTemplate{
				Manifests: []workapiv1.Manifest{
					{
						runtime.RawExtension{
							Object: mcPod,
						},
					},
				},
			},
		},
	}

	mcSTSManifestWork = &workapiv1.ManifestWork{
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
		Spec: workapiv1.ManifestWorkSpec{
			Workload: workapiv1.ManifestsTemplate{
				Manifests: []workapiv1.Manifest{
					{
						runtime.RawExtension{
							Object: mcSTS},
					},
				},
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
	g.Expect(err).NotTo(HaveOccurred())

	c := mgr.GetClient()
	rec, _ := NewReconciler(mgr, hubClusterConfig, mcName, explorer)

	ds := SetupManifestWorkSync(rec)

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

	// create the manifestWork on the hub
	mw := mcSVCManifestWork.DeepCopy()
	mwUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(mw)
	uc = &unstructured.Unstructured{}
	uc.SetUnstructuredContent(mwUC)
	uc.SetGroupVersionKind(manifestworkGVK)
	manifestWorkgvr := explorer.GVKGVRMap[manifestworkGVK]
	if _, err := hubDynamicClient.Resource(manifestWorkgvr).Namespace(mw.Namespace).Create(context.TODO(), uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = hubDynamicClient.Resource(manifestworkGVR).Namespace(mw.Namespace).Delete(context.TODO(), mw.Name, metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	if _, err := hubDynamicClient.Resource(manifestworkGVR).Namespace(mw.Namespace).Get(context.TODO(), mcSVCManifestWork.Name, metav1.GetOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}
	// wait for the manifestWork sync to come through on hub
	<-ds.(ManifestWorkSync).createCh
	<-ds.(ManifestWorkSync).updateCh

	// validate the annotation/labels created on the MC object
	newSVC, _ := mcDynamicClient.Resource(svcGVR).Namespace(svc.Namespace).Get(context.TODO(), svc.Name, metav1.GetOptions{})
	annotations := newSVC.GetAnnotations()
	g.Expect(annotations[corev1alpha1.AnnotationHosting]).To(Equal(mw.Namespace + "/" + mw.Name))
	g.Expect(annotations[corev1alpha1.AnnotationHostingSubscription]).To(Equal("/"))
	g.Expect(annotations[corev1alpha1.AnnotationSyncSourceSubscription]).To(Equal("subnsdpl-/"))
}

func TestObjectWithOwnerReference(t *testing.T) {
	g := NewWithT(t)
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	explorer, err := utils.InitExplorer(hubClusterConfig, mgr.GetConfig(), mcName)
	g.Expect(err).NotTo(HaveOccurred())

	rec, _ := NewReconciler(mgr, hubClusterConfig, mcName, explorer)

	ds := SetupManifestWorkSync(rec)

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

	// create the manifestWork on the hub
	mw := mcPodManifestWork.DeepCopy()
	mwUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(mw)
	uc = &unstructured.Unstructured{}
	uc.SetUnstructuredContent(mwUC)
	uc.SetGroupVersionKind(manifestworkGVK)
	manifestWorkgvr := explorer.GVKGVRMap[manifestworkGVK]
	if _, err := hubDynamicClient.Resource(manifestWorkgvr).Namespace(mw.Namespace).Create(context.TODO(), uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = hubDynamicClient.Resource(manifestworkGVR).Namespace(mw.Namespace).Delete(context.TODO(), mw.Name, metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	if _, err := hubDynamicClient.Resource(manifestworkGVR).Namespace(mw.Namespace).Get(context.TODO(), mcPodManifestWork.Name, metav1.GetOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}
	// wait for the manifestWork sync to come through on hub
	<-ds.(ManifestWorkSync).createCh
	<-ds.(ManifestWorkSync).updateCh

	hubMw, err := hubDynamicClient.Resource(manifestworkGVR).Namespace(mw.Namespace).Get(context.TODO(), mcPodManifestWork.Name, metav1.GetOptions{})

	g.Expect(hubMw.GetKind()).To(Equal("ManifestWork"))

	// Check that the manifestwork points to the service, not a pod

	ucMw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(hubMw)

	manifests, found, err := unstructured.NestedSlice(ucMw, "spec", "workload", "manifests")
	if !found || err != nil {
		klog.Error("Cannot get manifests from manifestwork ", hubMw.GetNamespace()+"/"+hubMw.GetName())
		t.Fail()
	}
	manifest, ok := manifests[0].(map[string]interface{})
	if !ok {
		klog.Error("Cannot get manifest from manifestwork ", hubMw.GetNamespace()+"/"+hubMw.GetName())
		t.Fail()
	}

	kind, found, err := unstructured.NestedString(manifest, "kind")
	if !found || err != nil {
		klog.Error("Cannot get the wrapped object kind for manifestwork ", hubMw.GetNamespace()+"/"+hubMw.GetName())
		t.Fail()
	}

	name, found, err := unstructured.NestedString(manifest, "metadata", "name")
	if !found || err != nil {
		klog.Error("Cannot get the wrapped object name for manifestWork ", hubMw.GetNamespace()+"/"+hubMw.GetName())
		t.Fail()
	}

	namespace, found, err := unstructured.NestedString(manifest, "metadata", "namespace")
	if !found || err != nil {
		klog.Error("Cannot get the wrapped object namespace for manifestWork ", hubMw.GetNamespace()+"/"+hubMw.GetName())
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
	g.Expect(err).NotTo(HaveOccurred())

	rec, _ := NewReconciler(mgr, hubClusterConfig, mcName, explorer)

	ds := SetupManifestWorkSync(rec)

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

	// create the manifestWork on the hub
	mw := mcSTSManifestWork.DeepCopy()
	mwUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(mw)
	uc = &unstructured.Unstructured{}
	uc.SetUnstructuredContent(mwUC)
	uc.SetGroupVersionKind(manifestworkGVK)

	if _, err = hubDynamicClient.Resource(manifestworkGVR).Namespace(mw.Namespace).Create(context.TODO(), uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = hubDynamicClient.Resource(manifestworkGVR).Namespace(mw.Namespace).Delete(context.TODO(), mw.Name, metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	// wait for the manifestWork sync to come through on hub
	<-ds.(ManifestWorkSync).createCh
	<-ds.(ManifestWorkSync).updateCh

	// remove the object subscription anno
	newMw, _ := hubDynamicClient.Resource(manifestworkGVR).Namespace(mw.Namespace).Get(context.TODO(), mw.Name, metav1.GetOptions{})
	newSTS, _ := mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})

	stsAnnotations := newSTS.GetAnnotations()
	delete(stsAnnotations, corev1alpha1.AnnotationHosting)
	delete(stsAnnotations, corev1alpha1.AnnotationHostingSubscription)
	delete(stsAnnotations, corev1alpha1.AnnotationSyncSourceSubscription)
	newSTS.SetAnnotations(stsAnnotations)
	if _, err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Update(context.TODO(), newSTS, metav1.UpdateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	mwAnnotations := newMw.GetAnnotations()
	mwAnnotations[hdplv1alpha1.AnnotationHybridDiscovery] = hdplv1alpha1.HybridDiscoveryEnabled
	newMw.SetAnnotations(mwAnnotations)

	if _, err = hubDynamicClient.Resource(manifestworkGVR).Namespace(mw.Namespace).Update(context.TODO(), newMw, metav1.UpdateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}
	<-ds.(ManifestWorkSync).updateCh

	updatedSTS, _ := mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
	annotations := updatedSTS.GetAnnotations()
	g.Expect(annotations[corev1alpha1.AnnotationHosting]).To(Equal(mw.Namespace + "/" + mw.Name))
	g.Expect(annotations[corev1alpha1.AnnotationHostingSubscription]).To(Equal("/"))
	g.Expect(annotations[corev1alpha1.AnnotationSyncSourceSubscription]).To(Equal("subnsdpl-/"))
}

func TestRefreshObjectWithoutDiscovery(t *testing.T) {
	g := NewWithT(t)
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	explorer, err := utils.InitExplorer(hubClusterConfig, mgr.GetConfig(), mcName)
	g.Expect(err).NotTo(HaveOccurred())

	rec, _ := NewReconciler(mgr, hubClusterConfig, mcName, explorer)

	ds := SetupManifestWorkSync(rec)

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

	// create the manifestWork on the hub
	mw := mcSTSManifestWork.DeepCopy()
	mwUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(mw)
	uc = &unstructured.Unstructured{}
	uc.SetUnstructuredContent(mwUC)
	uc.SetGroupVersionKind(manifestworkGVK)

	if _, err = hubDynamicClient.Resource(manifestworkGVR).Namespace(mw.Namespace).Create(context.TODO(), uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = hubDynamicClient.Resource(manifestworkGVR).Namespace(mw.Namespace).Delete(context.TODO(), mw.Name, metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	// wait for the manifestWork sync to come through on hub
	<-ds.(ManifestWorkSync).createCh
	<-ds.(ManifestWorkSync).updateCh

	// remove the object subscription anno
	newMw, _ := hubDynamicClient.Resource(manifestworkGVR).Namespace(mw.Namespace).Get(context.TODO(), mw.Name, metav1.GetOptions{})
	newSTS, _ := mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})

	stsAnnotations := newSTS.GetAnnotations()
	delete(stsAnnotations, corev1alpha1.AnnotationHosting)
	delete(stsAnnotations, corev1alpha1.AnnotationHostingSubscription)
	delete(stsAnnotations, corev1alpha1.AnnotationSyncSourceSubscription)
	newSTS.SetAnnotations(stsAnnotations)
	if _, err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Update(context.TODO(), newSTS, metav1.UpdateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	mwAnnotations := newMw.GetAnnotations()
	newMw.SetAnnotations(mwAnnotations)

	if _, err = hubDynamicClient.Resource(manifestworkGVR).Namespace(mw.Namespace).Update(context.TODO(), newMw, metav1.UpdateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	// discovery flag has been turned into completed from enabled, so no changes on the resource expected
	updatedSTS, _ := mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
	annotations := updatedSTS.GetAnnotations()
	g.Expect(annotations[corev1alpha1.AnnotationHosting]).To(BeEmpty())
	g.Expect(annotations[corev1alpha1.AnnotationHostingSubscription]).To(BeEmpty())
	g.Expect(annotations[corev1alpha1.AnnotationSyncSourceSubscription]).To(BeEmpty())
}

func TestRefreshOwnershipChange(t *testing.T) {
	g := NewWithT(t)
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	explorer, err := utils.InitExplorer(hubClusterConfig, mgr.GetConfig(), mcName)
	g.Expect(err).NotTo(HaveOccurred())

	rec, _ := NewReconciler(mgr, hubClusterConfig, mcName, explorer)

	ds := SetupManifestWorkSync(rec)

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

	// create the manifestwork on the hub
	mw1 := mcSTSManifestWork.DeepCopy()
	mw1UC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(mw1)
	uc1 := &unstructured.Unstructured{}
	uc1.SetUnstructuredContent(mw1UC)
	uc1.SetGroupVersionKind(manifestworkGVK)

	if _, err = hubDynamicClient.Resource(manifestworkGVR).Namespace(mw1.Namespace).Create(context.TODO(), uc1, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = hubDynamicClient.Resource(manifestworkGVR).Namespace(mw1.Namespace).Delete(context.TODO(), mw1.Name, metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	// wait for the manifestwork sync to come through on hub
	<-ds.(ManifestWorkSync).createCh
	<-ds.(ManifestWorkSync).updateCh

	// create a new manifestwork pointing to the same resource
	mw2 := mcSTSManifestWork.DeepCopy()
	mw2.SetName(mcSTSName + "2")
	mw2UC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(mw2)
	uc2 := &unstructured.Unstructured{}
	uc2.SetUnstructuredContent(mw2UC)
	uc2.SetGroupVersionKind(manifestworkGVK)

	if _, err = hubDynamicClient.Resource(manifestworkGVR).Namespace(mw2.Namespace).Create(context.TODO(), uc2, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer func() {
		if err = hubDynamicClient.Resource(manifestworkGVR).Namespace(mw2.Namespace).Delete(context.TODO(), mw2.Name, metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	// wait for the manifestwork sync to come through on hub
	<-ds.(ManifestWorkSync).createCh

	updatedSTS, _ := mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
	annotations := updatedSTS.GetAnnotations()
	g.Expect(annotations[corev1alpha1.AnnotationHosting]).To(Equal(mw1.Namespace + "/" + mw1.Name))
	g.Expect(annotations[corev1alpha1.AnnotationHostingSubscription]).To(Equal("/"))
	g.Expect(annotations[corev1alpha1.AnnotationSyncSourceSubscription]).To(Equal("subnsdpl-/"))
}

func TestSyncManifestWork(t *testing.T) {
	g := NewWithT(t)
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	explorer, err := utils.InitExplorer(hubClusterConfig, mgr.GetConfig(), mcName)
	if err != nil {
		klog.Error("Failed to initialize the explorer")
		t.Fail()
	}

	rec, _ := NewReconciler(mgr, hubClusterConfig, mcName, explorer)

	ds := SetupManifestWorkSync(rec)

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

	err = SyncManifestWork(uc, explorer)
	g.Expect(err).ShouldNot(HaveOccurred())

	mwList, _ := hubDynamicClient.Resource(manifestworkGVR).Namespace(mcName).List(context.TODO(), metav1.ListOptions{})
	g.Expect(mwList.Items).To(HaveLen(1))

	mw, _ := locateManifestWorkForObject(uc, explorer)
	g.Expect(mw).NotTo(BeNil())

	err = SyncManifestWork(uc, explorer)
	g.Expect(err).ShouldNot(HaveOccurred())

	// sync again and expect existing manifestwork to be ignored
	uc, err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Get(context.TODO(), uc.GetName(), metav1.GetOptions{})
	if err != nil {
		klog.Error(err)
		t.Fail()
	}

	err = SyncManifestWork(uc, explorer)
	g.Expect(err).ShouldNot(HaveOccurred())

	mwList, _ = hubDynamicClient.Resource(manifestworkGVR).Namespace(mcName).List(context.TODO(), metav1.ListOptions{})
	g.Expect(mwList.Items).To(HaveLen(1))

	if err = hubDynamicClient.Resource(manifestworkGVR).Namespace(mw.Namespace).Delete(context.TODO(), mw.GetName(), metav1.DeleteOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}
	<-ds.(ManifestWorkSync).deleteCh
}

func TestManifestWorkCleanup(t *testing.T) {
	g := NewWithT(t)
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	explorer, err := utils.InitExplorer(hubClusterConfig, mgr.GetConfig(), mcName)
	if err != nil {
		klog.Error("Failed to initialize the explorer")
		t.Fail()
	}

	rec, _ := NewReconciler(mgr, hubClusterConfig, mcName, explorer)

	ds := SetupManifestWorkSync(rec)

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

	// create the manifestwork on the hub
	mwObj := mcSTSManifestWork.DeepCopy()
	mwUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(mwObj)
	uc = &unstructured.Unstructured{}
	uc.SetUnstructuredContent(mwUC)
	uc.SetGroupVersionKind(manifestworkGVK)

	if _, err = hubDynamicClient.Resource(manifestworkGVR).Namespace(mwObj.Namespace).Create(context.TODO(), uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	// wait for the manifestwork sync to come through on hub
	<-ds.(ManifestWorkSync).createCh
	<-ds.(ManifestWorkSync).updateCh

	// delete the sts
	if err = mcDynamicClient.Resource(stsGVR).Namespace(sts.Namespace).Delete(context.TODO(), sts.Name, metav1.DeleteOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	// trigger reconciliation on manifestwork and make sure it gets cleaned up
	newMw, _ := hubDynamicClient.Resource(manifestworkGVR).Namespace(mwObj.Namespace).Get(context.TODO(), mwObj.Name, metav1.GetOptions{})
	mwAnnotations := newMw.GetAnnotations()
	mwAnnotations[hdplv1alpha1.AnnotationHybridDiscovery] = hdplv1alpha1.HybridDiscoveryEnabled
	newMw.SetAnnotations(mwAnnotations)

	if _, err = hubDynamicClient.Resource(manifestworkGVR).Namespace(newMw.GetNamespace()).Update(context.TODO(), newMw, metav1.UpdateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}
	<-ds.(ManifestWorkSync).updateCh
	mw, _ := hubDynamicClient.Resource(manifestworkGVR).Namespace(mwObj.Namespace).Get(context.TODO(), mwObj.Name, metav1.GetOptions{})

	g.Expect(mw).To(BeNil())
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
	mwObj := mcSTSManifestWork.DeepCopy()
	mwUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(mwObj)
	uc := &unstructured.Unstructured{}
	uc.SetUnstructuredContent(mwUC)
	uc.SetGroupVersionKind(manifestworkGVK)

	if _, err = hubDynamicClient.Resource(manifestworkGVR).Namespace(mwObj.Namespace).Create(context.TODO(), uc, metav1.CreateOptions{}); err != nil {
		klog.Error(err)
		t.Fail()
	}

	defer rec.Stop()
}

func TestAdd(t *testing.T) {

	g := NewWithT(t)

	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	recError := Add(mgr, hubClusterConfig, mcName)

	if recError != nil {
		klog.Error(recError)
		t.Fail()

	}
}
