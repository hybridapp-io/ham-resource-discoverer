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
	"testing"

	sigappv1beta1 "github.com/kubernetes-sigs/application/pkg/apis/app/v1beta1"
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
)

var (
	webServiceName   = "wordpress-webserver-svc"
	webSTSName       = "wordpress-webserver"
	applicationName  = "wordpress-01"
	appLabelSelector = "app.kubernetes.io/name"

	mcNamespace   = "managedcluster"
	userNamespace = "default"

	cluster = types.NamespacedName{
		Name:      mcNamespace,
		Namespace: mcNamespace,
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

	wordpressAppGK1 = metav1.GroupKind{
		Group: "v1",
		Kind:  "Service",
	}
	wordpressAppGK2 = metav1.GroupKind{
		Group: "apps",
		Kind:  "StatefulSet",
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
			},
		},
	}
)

func TestApplicationDiscovery(t *testing.T) {
	g := NewWithT(t)
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	rec, _ := newReconciler(mgr, hubClusterConfig, cluster)
	as := SetupApplicationSync(rec)

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	as.start()
	defer as.stop()

	hubDynamicClient := rec.explorer.DynamicHubClient
	mcDynamicClient := rec.explorer.DynamicMCClient

	svcGVR := rec.explorer.GVKGVRMap[mcSVC.GroupVersionKind()]
	stsGVR := rec.explorer.GVKGVRMap[mcSTS.GroupVersionKind()]
	dplGVR := rec.explorer.GVKGVRMap[deployableGVK]
	appGVR := rec.explorer.GVKGVRMap[applicationGVK]

	svc := mcSVC.DeepCopy()
	svcUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(svc)
	uc := &unstructured.Unstructured{}
	uc.SetUnstructuredContent(svcUC)
	_, err = mcDynamicClient.Resource(svcGVR).Namespace(userNamespace).Create(uc, metav1.CreateOptions{})
	g.Expect(err).ShouldNot(HaveOccurred())
	defer func() {
		if err = mcDynamicClient.Resource(svcGVR).Namespace(userNamespace).Delete(svc.Name, &metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()
	sts := mcSTS.DeepCopy()
	stsUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(sts)
	uc = &unstructured.Unstructured{}
	uc.SetUnstructuredContent(stsUC)
	_, err = mcDynamicClient.Resource(stsGVR).Namespace(userNamespace).Create(uc, metav1.CreateOptions{})
	g.Expect(err).ShouldNot(HaveOccurred())
	defer func() {
		if err = mcDynamicClient.Resource(stsGVR).Namespace(userNamespace).Delete(sts.Name, &metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()
	app := mcApp.DeepCopy()
	appUC, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(app)
	uc = &unstructured.Unstructured{}
	uc.SetUnstructuredContent(appUC)
	_, err = mcDynamicClient.Resource(appGVR).Namespace(userNamespace).Create(uc, metav1.CreateOptions{})
	g.Expect(err).ShouldNot(HaveOccurred())
	defer func() {
		if err = mcDynamicClient.Resource(appGVR).Namespace(userNamespace).Delete(app.Name, &metav1.DeleteOptions{}); err != nil {
			klog.Error(err)
			t.Fail()
		}
	}()
	// wait on app reconcile on MC
	<-as.(ApplicationSync).createCh

	// retrieve deployablelist on hub
	dplList, _ := hubDynamicClient.Resource(dplGVR).Namespace(cluster.Namespace).List(metav1.ListOptions{})
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
		g.Expect(annotations[corev1alpha1.AnnotationDiscovered]).To(Equal("true"))

		annotations, _, _ = unstructured.NestedMap(dpl.Object, "spec", "template", "metadata", "annotations")
		g.Expect(annotations).To(Not(BeNil()))
		g.Expect(annotations[corev1alpha1.AnnotationDiscovered]).To(Equal("true"))

		labels, _, _ := unstructured.NestedMap(dpl.Object, "spec", "template", "metadata", "labels")
		g.Expect(labels).To(Not(BeNil()))
		g.Expect(labels[appLabelSelector]).To(Equal(applicationName))

	}

	// validate annotations created on app resources on managed cluster
	svcuc, _ := mcDynamicClient.Resource(svcGVR).Namespace(userNamespace).Get(svc.Name, metav1.GetOptions{})
	g.Expect(svcuc.GetAnnotations()[corev1alpha1.AnnotationDiscovered]).To(Equal("true"))

	stsuc, _ := mcDynamicClient.Resource(stsGVR).Namespace(userNamespace).Get(sts.Name, metav1.GetOptions{})
	g.Expect(stsuc.GetAnnotations()[corev1alpha1.AnnotationDiscovered]).To(Equal("true"))

}
