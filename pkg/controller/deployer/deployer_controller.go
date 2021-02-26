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

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	corev1alpha1 "github.com/hybridapp-io/ham-resource-discoverer/pkg/apis/core/v1alpha1"
)

const (
	trueCondition = "true"
)

// Add creates a new Deployer Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, hubconfig *rest.Config, clusterName string) error {

	hubclient, err := client.New(hubconfig, client.Options{})
	if err != nil {
		klog.Error("Failed to create client to hub with error:", err)
		return err
	}

	return add(mgr, newReconciler(mgr, hubclient, clusterName))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, hubclient client.Client, clusterName string) reconcile.Reconciler {
	return &ReconcileDeployer{Client: mgr.GetClient(), hubclient: hubclient, clusterName: clusterName}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("deployer-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Deployer
	err = c.Watch(&source.Kind{Type: &corev1alpha1.Deployer{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileDeployer implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileDeployer{}

// ReconcileDeployer reconciles a Deployer object
type ReconcileDeployer struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client.Client
	clusterName string
	hubclient   client.Client
}

// Reconcile reads that state of the cluster for a Deployer object and makes changes based on the state read
// and what is in the Deployer.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileDeployer) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	klog.Info("Reconciling Deployer for ", request)

	var err error

	deployer := &corev1alpha1.Deployer{}

	err = r.Get(context.TODO(), request.NamespacedName, deployer)
	if err != nil {
		if !errors.IsNotFound(err) {
			// unexpected error
			klog.Error("Failed to get deployer from api server with error:", err)
		}
	}

	// Still update deployerset in hub cluster namespace even without deployer Fetch the Deployer instance
	return r.reconcileDeployerSet()
}

func (r *ReconcileDeployer) reconcileDeployerSet() (reconcile.Result, error) {
	deployerlist := &corev1alpha1.DeployerList{}

	err := r.List(context.TODO(), deployerlist, &client.ListOptions{})
	if err != nil {
		klog.Error("Failed to list Deployers in managed cluster with error:", err)
		return reconcile.Result{}, nil
	}

	setspec := &corev1alpha1.DeployerSetSpec{}

	for _, deployer := range deployerlist.Items {
		desc := corev1alpha1.DeployerSpecDescriptor{}
		desc.Key = types.NamespacedName{Namespace: deployer.Namespace, Name: deployer.Name}.String()
		deployer.Spec.DeepCopyInto(&desc.Spec)
		setspec.Deployers = append(setspec.Deployers, desc)

		annotations := deployer.GetAnnotations()
		if isdefault, ok := annotations[corev1alpha1.IsDefaultDeployer]; ok {
			if isdefault == trueCondition {
				setspec.DefaultDeployer = desc.Key
			}
		}
	}

	deployerset := &corev1alpha1.DeployerSet{}

	err = r.hubclient.Get(context.TODO(), types.NamespacedName{Name: r.clusterName, Namespace: r.clusterName}, deployerset)
	if err != nil {
		if errors.IsNotFound(err) {
			deployerset.Name = r.clusterName
			deployerset.Namespace = r.clusterName
			setspec.DeepCopyInto(&deployerset.Spec)
			err = r.hubclient.Create(context.TODO(), deployerset, &client.CreateOptions{})
			if err != nil {
				klog.Info("Failed to create deployerset with error: ", err)
			}
		}
	} else {
		// if no deployers present on managed cluster, clean up the deployerset on the hub as well
		if len(deployerlist.Items) == 0 {
			if err = r.hubclient.Delete(context.TODO(), deployerset, &client.DeleteOptions{}); err != nil {
				klog.Error("Failed to delete deployerset in hub with error:", err)
			}
		} else {
			// update the deployerset on hub with the refreshed list of deployers
			setspec.DeepCopyInto(&deployerset.Spec)
			if err = r.hubclient.Update(context.TODO(), deployerset, &client.UpdateOptions{}); err != nil {
				klog.Error("Failed to update deployerset in hub with error:", err)
			}
		}
	}
	return reconcile.Result{}, nil
}
