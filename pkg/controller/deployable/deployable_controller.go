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
)

// Add creates a new Deployable Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, hubconfig *rest.Config, cluster types.NamespacedName) error {
	reconciler, err := newReconciler(mgr, hubconfig, cluster)
	if err != nil {
		klog.Error("Failed to create the deployer reconciler ", err)
		return err
	}
	reconciler.start()
	return nil
}

func newReconciler(mgr manager.Manager, hubconfig *rest.Config, cluster types.NamespacedName) (*ReconcileDeployable, error) {
	explorer, err := utils.InitExplorer(hubconfig, mgr.GetConfig(), cluster)
	if err != nil {
		klog.Error("Failed to create client explorer: ", err)
		return nil, err
	}

	var dynamicHubFactory = dynamicinformer.NewDynamicSharedInformerFactory(explorer.DynamicHubClient, resync)
	reconciler := &ReconcileDeployable{
		explorer: explorer,

		dynamicHubFactory: dynamicHubFactory,
	}
	return reconciler, nil
}

// blank assignment to verify that ReconcileDeployer implements ReconcileDeployableInterface
var _ ReconcileDeployableInterface = &ReconcileDeployable{}

type ReconcileDeployableInterface interface {
	start()
	syncCreateDeployable(obj interface{})
	syncUpdateDeployable(old interface{}, new interface{})
	syncRemoveDeployable(obj interface{})
	stop()
}

// ReconcileDeployable reconciles a Deployable object
type ReconcileDeployable struct {
	explorer          *utils.Explorer
	dynamicHubFactory dynamicinformer.DynamicSharedInformerFactory
	stopCh            chan struct{}
}

func (r *ReconcileDeployable) start() {
	if r.dynamicHubFactory == nil {
		return
	}
	r.stop()
	// generic explorer
	r.stopCh = make(chan struct{})

	if _, ok := r.explorer.GVKGVRMap[deployableGVK]; !ok {
		klog.Error("Failed to obtain gvr for deployable gvk:", deployableGVK.String())
		return
	}

	handler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(new interface{}) {
			r.syncCreateDeployable(new)
		},
		UpdateFunc: func(old, new interface{}) {
			r.syncUpdateDeployable(old, new)
		},
		DeleteFunc: func(old interface{}) {
			r.syncRemoveDeployable(old)
		},
	}

	r.dynamicHubFactory.ForResource(r.explorer.GVKGVRMap[deployableGVK]).Informer().AddEventHandler(handler)

	r.stopCh = make(chan struct{})
	r.dynamicHubFactory.Start(r.stopCh)
}

func (r *ReconcileDeployable) stop() {
	if r.stopCh != nil {
		r.dynamicHubFactory.WaitForCacheSync(r.stopCh)
		close(r.stopCh)
	}
	r.stopCh = nil
}

func (r *ReconcileDeployable) syncCreateDeployable(obj interface{}) {
	metaobj, err := meta.Accessor(obj)
	if err != nil {
		klog.Error("Failed to access object metadata for sync with error: ", err)
		return
	}

	//reconcile only deployables in the cluster namespace
	if metaobj.GetNamespace() != r.explorer.Cluster.Namespace {
		return
	}

	// exit if deployable does not have the hybrid-discovered annotation
	if annotation, ok := metaobj.GetAnnotations()[hdplv1alpha1.AnnotationHybridDiscovery]; !ok ||
		annotation != hdplv1alpha1.HybridDiscoveryEnabled {
		return
	}

	r.syncDeployable(metaobj)
}

func (r *ReconcileDeployable) syncUpdateDeployable(oldObj, newObj interface{}) {

	metaNew, err := meta.Accessor(newObj)
	if err != nil {
		klog.Error("Failed to access object metadata for sync with error: ", err)
		return
	}
	if metaNew.GetNamespace() != r.explorer.Cluster.Namespace {
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

func (r *ReconcileDeployable) syncRemoveDeployable(obj interface{}) {}

func (r *ReconcileDeployable) syncDeployable(metaobj metav1.Object) {

	tpl, err := locateObjectForDeployable(metaobj, r.explorer)
	if err != nil {
		klog.Error("Failed to retrieve the wrapped object for deployable ", metaobj.GetNamespace()+"/"+metaobj.GetName()+" with error: ", err)
		return
	}
	if tpl == nil {
		klog.Info("Cleaning up orphaned deployable ", metaobj.GetNamespace()+"/"+metaobj.GetName())
		// remove deployable from hub
		gvr := r.explorer.GVKGVRMap[deployableGVK]
		err = r.explorer.DynamicHubClient.Resource(gvr).Namespace(metaobj.GetNamespace()).Delete(metaobj.GetName(), &metav1.DeleteOptions{})
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
	if err = updateDeployableAndObject(dpl, tpl, r.explorer); err != nil {
		klog.Error("Cannot update deployable ", metaobj.GetNamespace()+"/"+metaobj.GetName()+" with error: ", err)
		return
	}
}
