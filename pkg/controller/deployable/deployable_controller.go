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
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/apps/v1"

	hdplv1alpha1 "github.com/hybridapp-io/ham-deployable-operator/pkg/apis/core/v1alpha1"
	"github.com/hybridapp-io/ham-resource-discoverer/pkg/utils"
)

var (
	resync        = 20 * time.Minute
	deployableGVK = schema.GroupVersionKind{
		Group:   dplv1.SchemeGroupVersion.Group,
		Version: dplv1.SchemeGroupVersion.Version,
		Kind:    "Deployable",
	}

	deployableGVR = schema.GroupVersionResource{
		Group:    dplv1.SchemeGroupVersion.Group,
		Version:  dplv1.SchemeGroupVersion.Version,
		Resource: "deployables",
	}
)

func Add(mgr manager.Manager, hubconfig *rest.Config, cluster types.NamespacedName) error {

	explorer, err := utils.InitExplorer(hubconfig, mgr.GetConfig(), cluster)
	if err != nil {
		klog.Error("Failed to initialize the explorer")
		return err
	}

	reconciler, err := NewReconciler(mgr, hubconfig, cluster, explorer)
	if err != nil {
		klog.Error("Failed to create the deployer reconciler ", err)
		return err
	}
	reconciler.Start()
	return nil
}

func NewReconciler(mgr manager.Manager, hubconfig *rest.Config, cluster types.NamespacedName,
	explorer *utils.Explorer) (*ReconcileDeployable, error) {
	var dynamicHubFactory = dynamicinformer.NewFilteredDynamicSharedInformerFactory(explorer.DynamicHubClient, resync,
		cluster.Namespace, nil)
	reconciler := &ReconcileDeployable{
		Explorer:          explorer,
		DynamicHubFactory: dynamicHubFactory,
	}
	return reconciler, nil
}

// blank assignment to verify that ReconcileDeployer implements ReconcileDeployableInterface
var _ ReconcileDeployableInterface = &ReconcileDeployable{}

type ReconcileDeployableInterface interface {
	Start()
	SyncCreateDeployable(obj interface{})
	SyncUpdateDeployable(old interface{}, new interface{})
	SyncRemoveDeployable(obj interface{})
	Stop()
}

// ReconcileDeployable reconciles a Deployable object
type ReconcileDeployable struct {
	Explorer          *utils.Explorer
	DynamicHubFactory dynamicinformer.DynamicSharedInformerFactory
	StopCh            chan struct{}
}

func (r *ReconcileDeployable) Start() {
	if r.DynamicHubFactory == nil {
		return
	}
	r.Stop()
	// generic explorer
	r.StopCh = make(chan struct{})

	handler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(new interface{}) {
			r.SyncCreateDeployable(new)
		},
		UpdateFunc: func(old, new interface{}) {
			r.SyncUpdateDeployable(old, new)
		},
		DeleteFunc: func(old interface{}) {
			r.SyncRemoveDeployable(old)
		},
	}

	r.DynamicHubFactory.ForResource(deployableGVR).Informer().AddEventHandler(handler)

	r.StopCh = make(chan struct{})
	r.DynamicHubFactory.Start(r.StopCh)
}

func (r *ReconcileDeployable) Stop() {
	if r.StopCh != nil {
		r.DynamicHubFactory.WaitForCacheSync(r.StopCh)
		close(r.StopCh)
	}
	r.StopCh = nil
}

func (r *ReconcileDeployable) SyncCreateDeployable(obj interface{}) {
	metaobj, err := meta.Accessor(obj)
	if err != nil {
		klog.Error("Failed to access object metadata for sync with error: ", err)
		return
	}

	//reconcile only deployables in the cluster namespace
	if metaobj.GetNamespace() != r.Explorer.Cluster.Namespace {
		return
	}

	// exit if deployable does not have the hybrid-discovered annotation
	if annotation, ok := metaobj.GetAnnotations()[hdplv1alpha1.AnnotationHybridDiscovery]; !ok ||
		annotation != hdplv1alpha1.HybridDiscoveryEnabled {
		return
	}

	r.syncDeployable(metaobj)
}

func (r *ReconcileDeployable) SyncUpdateDeployable(oldObj, newObj interface{}) {

	metaNew, err := meta.Accessor(newObj)
	if err != nil {
		klog.Error("Failed to access object metadata for sync with error: ", err)
		return
	}
	if metaNew.GetNamespace() != r.Explorer.Cluster.Namespace {
		return
	}

	// exit if deployable does not have the hybrid-discovered annotation
	if annotation, ok := metaNew.GetAnnotations()[hdplv1alpha1.AnnotationHybridDiscovery]; !ok ||
		annotation != hdplv1alpha1.HybridDiscoveryEnabled {
		return
	}

	metaOld, err := meta.Accessor(oldObj)
	if err != nil {
		klog.Error("Failed to access object metadata for sync with error: ", err)
		return
	}

	ucOld := oldObj.(*unstructured.Unstructured)
	oldSpec, _, err := unstructured.NestedMap(ucOld.Object, "spec")
	if err != nil {
		klog.Error("Failed to retrieve deployable spec with error: ", err)
		return
	}

	ucNew := newObj.(*unstructured.Unstructured)
	newSpec, _, err := unstructured.NestedMap(ucNew.Object, "spec")
	if err != nil {
		klog.Error("Failed to retrieve deployable spec with error: ", err)
		return
	}

	if equality.Semantic.DeepEqual(metaOld.GetLabels(), metaNew.GetLabels()) &&
		equality.Semantic.DeepEqual(metaOld.GetAnnotations(), metaNew.GetAnnotations()) &&
		equality.Semantic.DeepEqual(oldSpec, newSpec) {
		return
	}
	r.syncDeployable(metaNew)
}

func (r *ReconcileDeployable) SyncRemoveDeployable(obj interface{}) {}

func (r *ReconcileDeployable) syncDeployable(metaobj metav1.Object) {

	tpl, err := locateObjectForDeployable(metaobj, r.Explorer)
	if err != nil {
		klog.Error("Failed to retrieve the wrapped object for deployable ", metaobj.GetNamespace()+"/"+metaobj.GetName()+" with error: ", err)
		return
	}
	if tpl == nil {
		klog.Info("Cleaning up orphaned deployable ", metaobj.GetNamespace()+"/"+metaobj.GetName())
		// remove deployable from hub
		err = r.Explorer.DynamicHubClient.Resource(deployableGVR).Namespace(metaobj.GetNamespace()).Delete(context.TODO(), metaobj.GetName(), metav1.DeleteOptions{})
		if err != nil {
			klog.Error("Failed to delete orphaned deployable ", metaobj.GetNamespace()+"/"+metaobj.GetName())
		}
		return
	}

	dpl := &dplv1.Deployable{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(metaobj.(*unstructured.Unstructured).Object, dpl)
	if err != nil {
		klog.Error("Cannot convert unstructured to deployable ", metaobj.GetNamespace()+"/"+metaobj.GetName()+" with error: ", err)
		return
	}
	if err = updateDeployableAndObject(dpl, tpl, r.Explorer); err != nil {
		klog.Error("Cannot update deployable ", metaobj.GetNamespace()+"/"+metaobj.GetName()+" with error: ", err)
		return
	}
}
