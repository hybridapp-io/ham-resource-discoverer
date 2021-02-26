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

package deployer

import (
	"context"
	"testing"
	"time"

	corev1alpha1 "github.com/hybridapp-io/ham-resource-discoverer/pkg/apis/core/v1alpha1"
	. "github.com/onsi/gomega"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	kubevirtType = "kubevirt"

	deployerName      = "test-resource-discoverer"
	deployerNamespace = "default"
)

var (
	// A Deployer resource with metadata and spec.
	managedClusterDeployer = &corev1alpha1.Deployer{
		TypeMeta: metav1.TypeMeta{
			Kind: "Deployer",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        deployerName,
			Namespace:   deployerNamespace,
			Annotations: map[string]string{corev1alpha1.IsDefaultDeployer: "true"},
		},
		Spec: corev1alpha1.DeployerSpec{
			Type:  kubevirtType,
			Scope: apiextensions.ClusterScoped,
		},
	}
	managedClusterDeployer2 = &corev1alpha1.Deployer{
		TypeMeta: metav1.TypeMeta{
			Kind: "Deployer",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        deployerName + "2",
			Namespace:   deployerNamespace,
			Annotations: map[string]string{corev1alpha1.IsDefaultDeployer: "true"},
		},
		Spec: corev1alpha1.DeployerSpec{
			Type:  kubevirtType,
			Scope: apiextensions.ClusterScoped,
		},
	}

	// deployer object
	key = types.NamespacedName{
		Name:      deployerName,
		Namespace: deployerNamespace,
	}

	key2 = types.NamespacedName{
		Name:      deployerName + "2",
		Namespace: deployerNamespace,
	}

	expectedRequest  = reconcile.Request{NamespacedName: key}
	expectedRequest2 = reconcile.Request{NamespacedName: key2}

	timeout = time.Second * 2
)

func TestReconcile(t *testing.T) {
	g := NewWithT(t)

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	managedClusterClient := mgr.GetClient()
	hubClient, _ := client.New(hubClusterConfig, client.Options{})
	rec := newReconciler(mgr, hubClient, clusterNameOnHub)
	recFn, requests := SetupTestReconcile(rec)

	g.Expect(add(mgr, recFn)).NotTo(HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	nsHub := nsOnHub.DeepCopy()
	g.Expect(hubClient.Create(context.TODO(), nsHub)).To(Succeed())

	// Create the Deployer object and expect the Reconcile and Deployment to be created
	dep := managedClusterDeployer.DeepCopy()
	g.Expect(managedClusterClient.Create(context.TODO(), dep)).NotTo(HaveOccurred())

	// reconcile.Request
	g.Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

	// deployer in managed cluster
	deployerResource := &corev1alpha1.Deployer{}
	g.Expect(managedClusterClient.Get(context.TODO(), expectedRequest.NamespacedName, deployerResource)).NotTo(HaveOccurred())

	// don't use dep at this point for assertion, as the client.Create nulled out the TypeMeta.
	g.Expect(deployerResource.TypeMeta.Kind).To(Equal(managedClusterDeployer.TypeMeta.Kind))
	g.Expect(deployerResource.ObjectMeta.Name).To(Equal(managedClusterDeployer.ObjectMeta.Name))
	g.Expect(deployerResource.ObjectMeta.Namespace).To(Equal(managedClusterDeployer.ObjectMeta.Namespace))
	g.Expect(deployerResource.Spec).To(Equal(managedClusterDeployer.Spec))

	if err = managedClusterClient.Delete(context.TODO(), dep); err != nil {
		t.Fail()
	}
	g.Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
}

func TestDeployersetCreatedOnHub(t *testing.T) {
	g := NewWithT(t)

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	managedClusterClient := mgr.GetClient()
	innerHubClient, _ := client.New(hubClusterConfig, client.Options{})
	hubClusterClient := SetupHubClient(innerHubClient)

	rec := newReconciler(mgr, hubClusterClient, clusterNameOnHub)
	recFn, requests := SetupTestReconcile(rec)

	g.Expect(add(mgr, recFn)).NotTo(HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	dep := managedClusterDeployer.DeepCopy()
	g.Expect(managedClusterClient.Create(context.TODO(), dep)).NotTo(HaveOccurred())
	defer func() {
		if err = managedClusterClient.Delete(context.TODO(), dep); err != nil {
			klog.Error(err)
			t.Fail()
		}
		// wait for deployerset to be gone
		<-hubClusterClient.(HubClient).deleteCh
	}()
	g.Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

	// wait for the hub resource to come through
	<-hubClusterClient.(HubClient).createCh

	// deployerset in hub cluster
	deployersetResource := &corev1alpha1.DeployerSet{}
	g.Expect(hubClusterClient.Get(context.TODO(), types.NamespacedName{Name: clusterNameOnHub,
		Namespace: clusterNameOnHub}, deployersetResource)).NotTo(HaveOccurred())
	g.Expect(deployersetResource.ObjectMeta.Name).To(Equal(clusterNameOnHub))
	g.Expect(deployersetResource.ObjectMeta.Namespace).To(Equal(clusterNamespaceOnHub))
	g.Expect(deployersetResource.Spec.DefaultDeployer).To(Equal(deployerNamespace + "/" + deployerName))
	g.Expect(deployersetResource.Spec.Deployers).To(HaveLen(1))
	g.Expect(deployersetResource.Spec.Deployers[0].Spec).To(Equal(managedClusterDeployer.Spec))
}

func TestDeployersetRemovedFromHub(t *testing.T) {
	g := NewWithT(t)

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	managedClusterClient := mgr.GetClient()
	innerHubClient, _ := client.New(hubClusterConfig, client.Options{})
	hubClusterClient := SetupHubClient(innerHubClient)

	rec := newReconciler(mgr, hubClusterClient, clusterNameOnHub)
	recFn, requests := SetupTestReconcile(rec)

	g.Expect(add(mgr, recFn)).NotTo(HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	dep := managedClusterDeployer.DeepCopy()
	g.Expect(managedClusterClient.Create(context.TODO(), dep)).NotTo(HaveOccurred())

	g.Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
	// wait for the hub resource to come through
	<-hubClusterClient.(HubClient).createCh

	if err = managedClusterClient.Delete(context.TODO(), dep); err != nil {
		klog.Error(err)
		t.Fail()
	}

	<-hubClusterClient.(HubClient).deleteCh

	// deployerset in hub cluster
	deployersetResource := &corev1alpha1.DeployerSet{}
	err = hubClusterClient.Get(context.TODO(), types.NamespacedName{Name: clusterNameOnHub, Namespace: clusterNameOnHub}, deployersetResource)
	g.Expect(errors.IsNotFound(err)).To(BeTrue())
}

func TestDeployerSetUpdateWithRefreshedDeployerList(t *testing.T) {

	g := NewWithT(t)

	mgr, err := manager.New(managedClusterConfig, manager.Options{MetricsBindAddress: "0"})

	g.Expect(err).NotTo(HaveOccurred())

	managedClusterClient := mgr.GetClient()
	innerHubClient, _ := client.New(hubClusterConfig, client.Options{})
	hubClusterClient := SetupHubClient(innerHubClient)

	rec := newReconciler(mgr, hubClusterClient, clusterNameOnHub)
	recFn, requests := SetupTestReconcile(rec)

	g.Expect(add(mgr, recFn)).NotTo(HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	dep := managedClusterDeployer.DeepCopy()
	dep2 := managedClusterDeployer2.DeepCopy()

	g.Expect(managedClusterClient.Create(context.TODO(), dep)).NotTo(HaveOccurred())

	g.Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

	g.Expect(managedClusterClient.Create(context.TODO(), dep2)).NotTo(HaveOccurred())
	g.Eventually(requests, timeout).Should(Receive(Equal(expectedRequest2)))

	if err = managedClusterClient.Delete(context.TODO(), dep); err != nil {
		klog.Error(err)
		t.Fail()
	}

	if err = managedClusterClient.Delete(context.TODO(), dep2); err != nil {
		klog.Error(err)
		t.Fail()
	}

}
