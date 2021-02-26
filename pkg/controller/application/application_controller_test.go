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

package application

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	hdplv1alpha1 "github.com/hybridapp-io/ham-deployable-operator/pkg/apis/core/v1alpha1"
	"github.com/hybridapp-io/ham-resource-discoverer/pkg/utils"
	sigappv1beta1 "github.com/kubernetes-sigs/application/pkg/apis/app/v1beta1"
	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/apps/v1"
)

var (
	deployableGVR = schema.GroupVersionResource{
		Group:    dplv1.SchemeGroupVersion.Group,
		Version:  dplv1.SchemeGroupVersion.Version,
		Resource: "deployables",
	}

	webServiceName   = "wordpress-webserver-svc"
	webSTSName       = "wordpress-webserver"
	webCMName        = "wordpress-configmap"
	applicationName  = "wordpress-01"
	appLabelSelector = "app.kubernetes.io/name"

	mcName           = "managedcluster"
	userNamespace    = "default"
	clusterNamespace = "clusterns"

	clusterNS = v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterNamespace,
		},
	}

	mcNS = v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: mcName,
		},
	}

	webServicePort = v1.ServicePort{
		Port: 3306,
	}

	mcSVC = &v1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      webServiceName,
			Namespace: userNamespace,
			Labels:    map[string]string{appLabelSelector: applicationName},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{webServicePort},
		},
	}

	mcSTS = &apps.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "StatefulSet",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      webSTSName,
			Namespace: userNamespace,
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

	mcCM = &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      webCMName,
			Namespace: clusterNamespace,
			Labels:    map[string]string{appLabelSelector: applicationName},
		},
		Data: map[string]string{"myconfig": "foo"},
	}

	wordpressAppGK1 = metav1.GroupKind{
		Group: "v1",
		Kind:  "Service",
	}
	wordpressAppGK2 = metav1.GroupKind{
		Group: "apps",
		Kind:  "StatefulSet",
	}
	wordpressAppGK3 = metav1.GroupKind{
		Group: "v1",
		Kind:  "ConfigMap",
	}

	mcApp = &sigappv1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "app.k8s.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      applicationName,
			Namespace: userNamespace,
			Labels:    map[string]string{appLabelSelector: applicationName},
		},
		Spec: sigappv1beta1.ApplicationSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{appLabelSelector: applicationName},
			},
			ComponentGroupKinds: []metav1.GroupKind{
				wordpressAppGK1,
				wordpressAppGK2,
				wordpressAppGK3,
			},
		},
	}
)

func TestApplicationDiscovery(t *testing.T) {
	g := NewWithT(t)
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	explorer, err := utils.InitExplorer(hubClusterConfig, mgr.GetConfig(), mcName)

	c := mgr.GetClient()
	rec, _ := NewReconciler(mgr, hubClusterConfig, mcName, explorer)
	as := SetupApplicationSync(rec)

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	as.Start()
	defer as.Stop()

	hubDynamicClient := explorer.DynamicHubClient
	mcDynamicClient := explorer.DynamicMCClient

	svcGVR := explorer.GVKGVRMap[mcSVC.GroupVersionKind()]
	stsGVR := explorer.GVKGVRMap[mcSTS.GroupVersionKind()]
	cmGVR := explorer.GVKGVRMap[mcCM.GroupVersionKind()]
	appGVR := explorer.GVKGVRMap[applicationGVK]

	cNS := clusterNS.DeepCopy()
	g.Expect(c.Create(context.TODO(), cNS)).To(Succeed())

	mcNS := mcNS.DeepCopy()

	nsGVR := schema.GroupVersionResource{
		Version:  "v1",
		Resource: "namespaces",
	}
	ucUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(mcNS)
	ucNS := &unstructured.Unstructured{}
	ucNS.SetUnstructuredContent(ucUC)
	_, err = hubDynamicClient.Resource(nsGVR).Create(context.TODO(), ucNS, metav1.CreateOptions{})
	g.Expect(err).ShouldNot(HaveOccurred())

	svc := mcSVC.DeepCopy()
	svcUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(svc)
	uc := &unstructured.Unstructured{}
	uc.SetUnstructuredContent(svcUC)
	_, err = mcDynamicClient.Resource(svcGVR).Namespace(userNamespace).Create(context.TODO(), uc, metav1.CreateOptions{})
	g.Expect(err).ShouldNot(HaveOccurred())
	defer func() {
		if err = mcDynamicClient.Resource(svcGVR).Namespace(userNamespace).Delete(context.TODO(), svc.Name, metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	sts := mcSTS.DeepCopy()
	stsUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(sts)
	uc = &unstructured.Unstructured{}
	uc.SetUnstructuredContent(stsUC)
	_, err = mcDynamicClient.Resource(stsGVR).Namespace(userNamespace).Create(context.TODO(), uc, metav1.CreateOptions{})
	g.Expect(err).ShouldNot(HaveOccurred())
	defer func() {
		if err = mcDynamicClient.Resource(stsGVR).Namespace(userNamespace).Delete(context.TODO(), sts.Name, metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	cm := mcCM.DeepCopy()
	cmUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(cm)
	uc = &unstructured.Unstructured{}
	uc.SetUnstructuredContent(cmUC)
	_, err = mcDynamicClient.Resource(cmGVR).Namespace(clusterNamespace).Create(context.TODO(), uc, metav1.CreateOptions{})
	g.Expect(err).ShouldNot(HaveOccurred())
	defer func() {
		if err = mcDynamicClient.Resource(cmGVR).Namespace(clusterNamespace).Delete(context.TODO(), cm.Name, metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	app := mcApp.DeepCopy()
	app.Annotations = make(map[string]string)
	app.Annotations[hdplv1alpha1.AnnotationHybridDiscovery] = hdplv1alpha1.HybridDiscoveryEnabled
	appUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(app)
	uc = &unstructured.Unstructured{}
	uc.SetUnstructuredContent(appUC)
	uc, err = mcDynamicClient.Resource(appGVR).Namespace(userNamespace).Create(context.TODO(), uc, metav1.CreateOptions{})
	g.Expect(err).ShouldNot(HaveOccurred())
	defer func() {
		if err = mcDynamicClient.Resource(appGVR).Namespace(userNamespace).Delete(context.TODO(), app.Name, metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()
	// wait on app reconcile on MC
	<-as.(ApplicationSync).CreateCh

	// retrieve deployablelist on hub
	dplList, _ := hubDynamicClient.Resource(deployableGVR).Namespace(mcName).List(context.TODO(), metav1.ListOptions{})
	// expect only 2 deployables, as the configmap was created in another namespace
	g.Expect(dplList.Items).To(HaveLen(2))
	kinds := [2]string{"Service", "StatefulSet"}

	// validate deployables on hub
	for _, dpl := range dplList.Items {
		// spec containing the template
		tpl, _, _ := unstructured.NestedMap(dpl.Object, "spec", "template")
		g.Expect(tpl).To(Not(BeNil()))
		g.Expect(tpl["kind"]).To(BeElementOf(kinds))

		annotations, _, _ := unstructured.NestedMap(dpl.Object, "metadata", "annotations")
		g.Expect(annotations).To(Not(BeNil()))
		g.Expect(annotations[hdplv1alpha1.AnnotationHybridDiscovery]).To(Equal(hdplv1alpha1.HybridDiscoveryEnabled))

		labels, _, _ := unstructured.NestedMap(dpl.Object, "spec", "template", "metadata", "labels")
		g.Expect(labels).To(Not(BeNil()))
		g.Expect(labels[appLabelSelector]).To(Equal(applicationName))
	}

	// update application by adding a random annotation.
	annotations, _, _ := unstructured.NestedMap(uc.Object, "metadata", "annotations")
	annotations["random_annotation_name"] = "random_annotation_value"
	if err = unstructured.SetNestedMap(uc.Object, annotations, "metadata", "annotations"); err != nil {
		klog.Error(err)
		t.Fail()
	}

	uc, err = mcDynamicClient.Resource(appGVR).Namespace(userNamespace).Update(context.TODO(), uc, metav1.UpdateOptions{})
	g.Expect(err).ShouldNot(HaveOccurred())
	// wait on app reconcile on MC
	<-as.(ApplicationSync).UpdateCh

	// update application by adding AnnotationClusterScope.
	annotations, _, _ = unstructured.NestedMap(uc.Object, "metadata", "annotations")
	annotations[hdplv1alpha1.AnnotationClusterScope] = "true"
	if err = unstructured.SetNestedMap(uc.Object, annotations, "metadata", "annotations"); err != nil {
		klog.Error(err)
		t.Fail()
	}
	if err = unstructured.SetNestedField(uc.Object, true, "spec", "addOwnerRef"); err != nil {
		klog.Error(err)
		t.Fail()
	}

	uc, err = mcDynamicClient.Resource(appGVR).Namespace(userNamespace).Update(context.TODO(), uc, metav1.UpdateOptions{})
	g.Expect(err).ShouldNot(HaveOccurred())

	// wait on app reconcile on MC
	<-as.(ApplicationSync).UpdateCh

	// retrieve deployablelist on hub
	dplList, _ = hubDynamicClient.Resource(deployableGVR).Namespace(mcName).List(context.TODO(), metav1.ListOptions{})
	// expect 3 deployables, as the configmap is covered now by AnnotationClusterScope
	g.Expect(dplList.Items).To(HaveLen(3))

	// update application by removing the discovery annotation
	annotations, _, _ = unstructured.NestedMap(uc.Object, "metadata", "annotations")
	delete(annotations, hdplv1alpha1.AnnotationHybridDiscovery)
	if err = unstructured.SetNestedMap(uc.Object, annotations, "metadata", "annotations"); err != nil {
		klog.Error(err)
		t.Fail()
	}

	_, err = mcDynamicClient.Resource(appGVR).Namespace(userNamespace).Update(context.TODO(), uc, metav1.UpdateOptions{})
	g.Expect(err).ShouldNot(HaveOccurred())

	// wait on app reconcile on MC
	<-as.(ApplicationSync).UpdateCh

}

func TestGenericControllerReconcile(t *testing.T) {
	g := NewWithT(t)
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	explorer, err := utils.InitExplorer(hubClusterConfig, mgr.GetConfig(), mcName)

	rec, _ := NewReconciler(mgr, hubClusterConfig, mcName, explorer)

	mcDynamicClient := explorer.DynamicMCClient
	appGVR := explorer.GVKGVRMap[applicationGVK]

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	rec.Start()
	app := mcApp.DeepCopy()
	app.Annotations = make(map[string]string)
	app.Annotations[hdplv1alpha1.AnnotationHybridDiscovery] = hdplv1alpha1.HybridDiscoveryEnabled
	appUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(app)
	uc := &unstructured.Unstructured{}
	uc.SetUnstructuredContent(appUC)
	_, err = mcDynamicClient.Resource(appGVR).Namespace(userNamespace).Create(context.TODO(), uc, metav1.CreateOptions{})
	g.Expect(err).ShouldNot(HaveOccurred())
	defer func() {
		if err = mcDynamicClient.Resource(appGVR).Namespace(userNamespace).Delete(context.TODO(), app.Name, metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()

	defer rec.Stop()
}
