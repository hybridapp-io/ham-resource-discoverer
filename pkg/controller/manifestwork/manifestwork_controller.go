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

// extra comment
import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	workapiv1 "github.com/open-cluster-management/api/work/v1"

	hdplv1alpha1 "github.com/hybridapp-io/ham-deployable-operator/pkg/apis/core/v1alpha1"
	"github.com/hybridapp-io/ham-resource-discoverer/pkg/utils"
)

var (
	resync          = 20 * time.Minute
	manifestworkGVK = schema.GroupVersionKind{
		Group:   workapiv1.SchemeGroupVersion.Group,
		Version: workapiv1.SchemeGroupVersion.Version,
		Kind:    "ManifestWork",
	}

	manifestworkGVR = schema.GroupVersionResource{
		Group:    workapiv1.SchemeGroupVersion.Group,
		Version:  workapiv1.SchemeGroupVersion.Version,
		Resource: "manifestworks",
	}
)

func Add(mgr manager.Manager, hubconfig *rest.Config, clusterName string) error {
	explorer, err := utils.InitExplorer(hubconfig, mgr.GetConfig(), clusterName)
	if err != nil {
		klog.Error("Failed to initialize the explorer")
		return err
	}

	reconciler, err := NewReconciler(mgr, hubconfig, clusterName, explorer)
	if err != nil {
		klog.Error("Failed to create the deployer reconciler ", err)
		return err
	}
	reconciler.Start()
	return nil
}

func NewReconciler(mgr manager.Manager, hubconfig *rest.Config, clusterName string,
	explorer *utils.Explorer) (*ReconcileManifestWork, error) {
	var dynamicHubFactory = dynamicinformer.NewFilteredDynamicSharedInformerFactory(explorer.DynamicHubClient, resync,
		clusterName, nil)
	reconciler := &ReconcileManifestWork{
		Explorer:          explorer,
		DynamicHubFactory: dynamicHubFactory,
	}
	return reconciler, nil
}

// blank assignment to verify that ReconcileManifestWork implements ReconcileManifestWorkInterface
var _ ReconcileManifestWorkInterface = &ReconcileManifestWork{}

type ReconcileManifestWorkInterface interface {
	Start()
	SyncCreateManifestWork(obj interface{})
	SyncUpdateManifestWork(old interface{}, new interface{})
	SyncRemoveManifestWork(obj interface{})
	Stop()
}

// ReconcileManifestWork reconciles a ManifestWork object
type ReconcileManifestWork struct {
	Explorer          *utils.Explorer
	DynamicHubFactory dynamicinformer.DynamicSharedInformerFactory
	StopCh            chan struct{}
}

func (r *ReconcileManifestWork) Start() {
	if r.DynamicHubFactory == nil {
		return
	}
	r.Stop()
	// generic explorer
	r.StopCh = make(chan struct{})

	handler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(new interface{}) {
			r.SyncCreateManifestWork(new)
		},
		UpdateFunc: func(old, new interface{}) {
			r.SyncUpdateManifestWork(old, new)
		},
		DeleteFunc: func(old interface{}) {
			r.SyncRemoveManifestWork(old)
		},
	}

	r.DynamicHubFactory.ForResource(manifestworkGVR).Informer().AddEventHandler(handler)

	r.StopCh = make(chan struct{})
	r.DynamicHubFactory.Start(r.StopCh)
}

func (r *ReconcileManifestWork) Stop() {
	if r.StopCh != nil {
		r.DynamicHubFactory.WaitForCacheSync(r.StopCh)
		close(r.StopCh)
	}
	r.StopCh = nil
}

func (r *ReconcileManifestWork) SyncCreateManifestWork(obj interface{}) {
	metaobj, err := meta.Accessor(obj)
	if err != nil {
		klog.Error("Failed to access object metadata for sync with error: ", err)
		return
	}

	//reconcile only manifestworks in the cluster namespace
	if metaobj.GetNamespace() != r.Explorer.ClusterName {
		return
	}

	// exit if manifestwork does not have the hybrid-discovered annotation
	if annotation, ok := metaobj.GetAnnotations()[hdplv1alpha1.AnnotationHybridDiscovery]; !ok ||
		annotation != hdplv1alpha1.HybridDiscoveryEnabled {
		return
	}

	r.syncManifestWork(metaobj)
}

func (r *ReconcileManifestWork) SyncUpdateManifestWork(oldObj, newObj interface{}) {

	metaNew, err := meta.Accessor(newObj)
	if err != nil {
		klog.Error("Failed to access object metadata for sync with error: ", err)
		return
	}
	if metaNew.GetNamespace() != r.Explorer.ClusterName {
		return
	}

	// exit if manifestwork does not have the hybrid-discovered annotation
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
		klog.Error("Failed to retrieve manifestwork spec with error: ", err)
		return
	}

	ucNew := newObj.(*unstructured.Unstructured)
	newSpec, _, err := unstructured.NestedMap(ucNew.Object, "spec")
	if err != nil {
		klog.Error("Failed to retrieve manifestwork spec with error: ", err)
		return
	}

	if equality.Semantic.DeepEqual(metaOld.GetLabels(), metaNew.GetLabels()) &&
		equality.Semantic.DeepEqual(metaOld.GetAnnotations(), metaNew.GetAnnotations()) &&
		equality.Semantic.DeepEqual(oldSpec, newSpec) {
		return
	}
	r.syncManifestWork(metaNew)
}

func (r *ReconcileManifestWork) SyncRemoveManifestWork(obj interface{}) {}

func (r *ReconcileManifestWork) syncManifestWork(metaobj metav1.Object) {

	tpl, err := locateObjectForManifestWork(metaobj, r.Explorer)
	if err != nil {
		klog.Error("Failed to retrieve the wrapped object for manifestwork ", metaobj.GetNamespace()+"/"+metaobj.GetName()+" with error: ", err)
		return
	}
	if tpl == nil {
		klog.Info("Cleaning up orphaned manifestwork ", metaobj.GetNamespace()+"/"+metaobj.GetName())
		// remove manifestwork from hub
		err = r.Explorer.DynamicHubClient.Resource(manifestworkGVR).Namespace(metaobj.GetNamespace()).
			Delete(context.TODO(), metaobj.GetName(), metav1.DeleteOptions{})
		if err != nil {
			klog.Error("Failed to delete orphaned manifestwork ", metaobj.GetNamespace()+"/"+metaobj.GetName())
		}
		return
	}

	mw := &workapiv1.ManifestWork{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(metaobj.(*unstructured.Unstructured).Object, mw)
	if err != nil {
		klog.Error("Cannot convert unstructured to manifestwork ", metaobj.GetNamespace()+"/"+metaobj.GetName()+" with error: ", err)
		return
	}
	if err = updateManifestWorkAndObject(mw, tpl, r.Explorer); err != nil {
		klog.Error("Cannot update manifestwork ", metaobj.GetNamespace()+"/"+metaobj.GetName()+" with error: ", err)
		return
	}
}
