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
	"time"

	sigappv1beta1 "github.com/kubernetes-sigs/application/pkg/apis/app/v1beta1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/hybridapp-io/ham-resource-discoverer/pkg/controller/manifestwork"
	"github.com/hybridapp-io/ham-resource-discoverer/pkg/utils"

	hdplv1alpha1 "github.com/hybridapp-io/ham-deployable-operator/pkg/apis/core/v1alpha1"
)

const (
	packageInfoLogLevel = 3
)

var (
	resync         = 20 * time.Minute
	applicationGVK = schema.GroupVersionKind{
		Group:   sigappv1beta1.SchemeGroupVersion.Group,
		Version: sigappv1beta1.SchemeGroupVersion.Version,
		Kind:    "Application",
	}
)

// Add creates a newObj Deployable Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, hubconfig *rest.Config, clusterName string) error {
	explorer, err := utils.InitExplorer(hubconfig, mgr.GetConfig(), clusterName)
	if err != nil {
		klog.Error("Failed to initialize the explorer")
		return err
	}

	reconciler, err := NewReconciler(mgr, hubconfig, clusterName, explorer)
	if err != nil {
		klog.Error("Failed to create the application reconciler ", err)
		return err
	}
	reconciler.Start()
	return nil
}

func NewReconciler(mgr manager.Manager, hubconfig *rest.Config, clusterName string,
	explorer *utils.Explorer) (*ReconcileApplication, error) {
	var dynamicMCFactory = dynamicinformer.NewDynamicSharedInformerFactory(explorer.DynamicMCClient, resync)
	reconciler := &ReconcileApplication{
		Explorer:         explorer,
		DynamicMCFactory: dynamicMCFactory,
	}
	return reconciler, nil
}

// ReconcileApplication reconciles a Application object
type ReconcileApplication struct {
	Explorer         *utils.Explorer
	DynamicMCFactory dynamicinformer.DynamicSharedInformerFactory
	StopCh           chan struct{}
}

// blank assignment to verify that ReconcileDeployer implements ReconcileApplicationInterface
var _ ReconcileApplicationInterface = &ReconcileApplication{}

type ReconcileApplicationInterface interface {
	Start()
	SyncCreateApplication(newObj interface{})
	SyncUpdateApplication(oldObj interface{}, newObj interface{})
	SyncRemoveApplication(oldObj interface{})
	Stop()
}

func (r *ReconcileApplication) isAppDiscoveryEnabled(app *unstructured.Unstructured) bool {
	if _, enabled := app.GetAnnotations()[hdplv1alpha1.AnnotationHybridDiscovery]; !enabled ||
		app.GetAnnotations()[hdplv1alpha1.AnnotationHybridDiscovery] != hdplv1alpha1.HybridDiscoveryEnabled {
		return false
	}

	return true
}

func (r *ReconcileApplication) isDiscoveryClusterScoped(obj *unstructured.Unstructured) bool {
	if _, enabled := obj.GetAnnotations()[hdplv1alpha1.AnnotationClusterScope]; !enabled ||
		obj.GetAnnotations()[hdplv1alpha1.AnnotationClusterScope] != "true" {
		return false
	}

	return true
}

func (r *ReconcileApplication) Start() {
	r.Stop()

	if r.Explorer == nil || r.DynamicMCFactory == nil {
		return
	}
	// generic explorer
	r.StopCh = make(chan struct{})

	if _, ok := r.Explorer.GVKGVRMap[applicationGVK]; !ok {
		klog.Error("Failed to obtain gvr for application gvk:", applicationGVK.String())
		return
	}

	handler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(newObj interface{}) {
			r.SyncCreateApplication(newObj)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			r.SyncUpdateApplication(oldObj, newObj)
		},
		DeleteFunc: func(oldObj interface{}) {
			r.SyncRemoveApplication(oldObj)
		},
	}

	r.DynamicMCFactory.ForResource(r.Explorer.GVKGVRMap[applicationGVK]).Informer().AddEventHandler(handler)

	r.StopCh = make(chan struct{})
	r.DynamicMCFactory.Start(r.StopCh)
}

func (r *ReconcileApplication) Stop() {
	if r.StopCh != nil {
		r.DynamicMCFactory.WaitForCacheSync(r.StopCh)
		close(r.StopCh)
	}
	r.StopCh = nil
}

func (r *ReconcileApplication) SyncCreateApplication(newObj interface{}) {
	ucNew := newObj.(*unstructured.Unstructured)
	if !r.isAppDiscoveryEnabled(ucNew) {
		return
	}
	klog.V(packageInfoLogLevel).Info("Creating application ", newObj.(*unstructured.Unstructured).GetNamespace()+"/"+newObj.(*unstructured.Unstructured).GetName())
	if err := r.syncApplication(ucNew); err != nil {
		klog.Error("Could not reconcile application ", newObj.(*unstructured.Unstructured).GetName(), " on create with error ", err)
	}
}

func (r *ReconcileApplication) SyncUpdateApplication(oldObj, newObj interface{}) {

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
	// reconcile only if specs or discovered label have changed
	if !r.isAppDiscoveryEnabled(ucNew) {
		return
	}
	if equality.Semantic.DeepEqual(oldSpec, newSpec) {
		klog.V(packageInfoLogLevel).Info("Skip updating application ", ucNew.GetNamespace()+"/"+ucNew.GetName(), ". No changes detected")
		return
	}
	klog.V(packageInfoLogLevel).Info("Updating application ", ucNew.GetNamespace()+"/"+ucNew.GetName())
	if err := r.syncApplication(ucNew); err != nil {
		klog.Error("Could not reconcile application ", ucNew.GetNamespace()+"/"+ucNew.GetName(), " on update with error ", err)
	}
}

func (r *ReconcileApplication) SyncRemoveApplication(oldObj interface{}) {}

func (r *ReconcileApplication) syncApplication(obj *unstructured.Unstructured) error {

	// convert obj to Application
	app := &sigappv1beta1.Application{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, app)

	if err != nil {
		klog.Error("Failed to convert unstructured to application with error: ", err)
		return err
	}

	var appComponents = make(map[metav1.GroupKind]*unstructured.UnstructuredList)

	for _, componentKind := range app.Spec.ComponentGroupKinds {
		klog.Info("Processing application GK ", componentKind.String())
		for gvk, gvr := range r.Explorer.GVKGVRMap {

			if gvk.Kind == componentKind.Kind {
				// for v1 core group (which is the empty name group), application label selectors use v1 as group name
				if (gvk.Group == "" && gvk.Version == componentKind.Group) || (gvk.Group != "" && gvk.Group == componentKind.Group) {
					klog.V(packageInfoLogLevel).Info("Successfully found GVR ", gvr.String())

					var objlist *unstructured.UnstructuredList
					if r.isDiscoveryClusterScoped(obj) {
						// retrieve all components, cluster wide
						objlist, err = r.Explorer.DynamicMCClient.Resource(gvr).List(context.TODO(),
							metav1.ListOptions{LabelSelector: labels.Set(app.Spec.Selector.MatchLabels).String()})
						if len(objlist.Items) == 0 {
							// we still want to create the deployables for the resources we find on managed cluster ,
							// even though some kinds defined in the app may not have corresponding (satisfying the selector)
							// resources on managed cluster
							klog.Info("Could not find any managed cluster resources for application component with kind ", componentKind.String(), " cluster wide ")
						}

					} else {
						// retrieve only namespaced components
						objlist, err = r.Explorer.DynamicMCClient.Resource(gvr).Namespace(obj.GetNamespace()).List(context.TODO(),
							metav1.ListOptions{LabelSelector: labels.Set(app.Spec.Selector.MatchLabels).String()})
						if len(objlist.Items) == 0 {
							klog.Info("Could not find any managed cluster resources for application component with kind ",
								componentKind.String(), " in namespace ", obj.GetNamespace())
						}

					}
					if err != nil {
						klog.Error("Failed to retrieve the list of components based on selector ")
						return err
					}
					appComponents[componentKind] = objlist
					break
				}
			}
		}
	}

	// process the components on managed cluster and creates deployables on hub for them
	for _, objlist := range appComponents {
		for i := range objlist.Items {
			item := objlist.Items[i]
			klog.Info("Processing object ", item.GetName(), " in namespace ", item.GetNamespace(), " with kind ", item.GetKind())
			if err = manifestwork.SyncManifestWork(&item, r.Explorer); err != nil {
				klog.Error("Failed to sync resource ", item.GetNamespace()+"/"+item.GetName(), " with error ", err)
			}
		}
	}
	return nil
}
