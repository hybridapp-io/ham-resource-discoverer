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
	"time"

	sigappv1beta1 "github.com/kubernetes-sigs/application/pkg/apis/app/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	corev1alpha1 "github.com/hybridapp-io/ham-resource-discoverer/pkg/apis/core/v1alpha1"
	"github.com/hybridapp-io/ham-resource-discoverer/pkg/controller/deployable"
	"github.com/hybridapp-io/ham-resource-discoverer/pkg/utils"

	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/apps/v1"
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
	deployableGVK = schema.GroupVersionKind{
		Group:   dplv1.SchemeGroupVersion.Group,
		Version: dplv1.SchemeGroupVersion.Version,
		Kind:    "Deployable",
	}
)

// Add creates a newObj Deployable Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, hubconfig *rest.Config, cluster types.NamespacedName) error {
	reconciler, err := newReconciler(mgr, hubconfig, cluster)
	if err != nil {
		klog.Error("Failed to create the application reconciler ", err)
		return err
	}
	reconciler.start()
	return nil
}

func newReconciler(mgr manager.Manager, hubconfig *rest.Config, cluster types.NamespacedName) (*ReconcileApplication, error) {
	explorer, err := utils.InitExplorer(hubconfig, mgr.GetConfig(), cluster)
	if err != nil {
		klog.Error("Failed to create client explorer: ", err)
		return nil, err
	}
	var dynamicMCFactory = dynamicinformer.NewDynamicSharedInformerFactory(explorer.DynamicMCClient, resync)
	reconciler := &ReconcileApplication{
		explorer:         explorer,
		dynamicMCFactory: dynamicMCFactory,
	}
	return reconciler, nil
}

// ReconcileDeployable reconciles a Deployable object
type ReconcileApplication struct {
	explorer         *utils.Explorer
	dynamicMCFactory dynamicinformer.DynamicSharedInformerFactory
	stopCh           chan struct{}
}

// blank assignment to verify that ReconcileDeployer implements ReconcileDeployableInterface
var _ ReconcileApplicationInterface = &ReconcileApplication{}

type ReconcileApplicationInterface interface {
	start()
	syncCreateApplication(newObj interface{})
	syncUpdateApplication(oldObj interface{}, newObj interface{})
	syncRemoveApplication(oldObj interface{})
	stop()
}

func (r *ReconcileApplication) isAppDiscoveryEnabled(app *unstructured.Unstructured) bool {
	if _, enabled := app.GetAnnotations()[corev1alpha1.AnnotationDiscovered]; !enabled ||
		app.GetAnnotations()[corev1alpha1.AnnotationDiscovered] != corev1alpha1.DiscoveryEnabled {
		return false
	}

	return true
}

func (r *ReconcileApplication) start() {
	r.stop()

	if r.explorer == nil || r.dynamicMCFactory == nil {
		return
	}
	// generic explorer
	r.stopCh = make(chan struct{})

	if _, ok := r.explorer.GVKGVRMap[applicationGVK]; !ok {
		klog.Error("Failed to obtain gvr for application gvk:", applicationGVK.String())
		return
	}

	handler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(newObj interface{}) {
			r.syncCreateApplication(newObj)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			r.syncUpdateApplication(oldObj, newObj)
		},
		DeleteFunc: func(oldObj interface{}) {
			r.syncRemoveApplication(oldObj)
		},
	}

	r.dynamicMCFactory.ForResource(r.explorer.GVKGVRMap[applicationGVK]).Informer().AddEventHandler(handler)

	r.stopCh = make(chan struct{})
	r.dynamicMCFactory.Start(r.stopCh)
}

func (r *ReconcileApplication) stop() {
	if r.stopCh != nil {
		r.dynamicMCFactory.WaitForCacheSync(r.stopCh)
		close(r.stopCh)
	}
	r.stopCh = nil
}

func (r *ReconcileApplication) syncCreateApplication(newObj interface{}) {
	if err := r.syncApplication(newObj.(*unstructured.Unstructured)); err != nil {
		klog.Error("Could not reconcile application ", newObj.(*unstructured.Unstructured).GetName(), " on create with error ", err)
	}
}

func (r *ReconcileApplication) syncUpdateApplication(oldObj, newObj interface{}) {
	if err := r.syncApplication(newObj.(*unstructured.Unstructured)); err != nil {
		klog.Error("Could not reconcile application ", newObj.(*unstructured.Unstructured).GetName(), " on update with error ", err)
	}
}

func (r *ReconcileApplication) syncRemoveApplication(oldObj interface{}) {

}

func (r *ReconcileApplication) syncApplication(obj *unstructured.Unstructured) error {

	if !r.isAppDiscoveryEnabled(obj) {
		return nil
	}
	// convert obj to Application
	app := &sigappv1beta1.Application{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, app)

	if err != nil {
		klog.Error("Failed to convert unstructured to application with error: ", err)
		return err
	}
	var appComponents map[metav1.GroupKind]*unstructured.UnstructuredList = make(map[metav1.GroupKind]*unstructured.UnstructuredList)

	for _, componentKind := range app.Spec.ComponentGroupKinds {
		klog.Info("Processing application GK ", componentKind.String())
		for gvk, gvr := range r.explorer.GVKGVRMap {

			if gvk.Kind == componentKind.Kind {
				// for v1 core group (which is the empty name group), application label selectors use v1 as group name
				if (gvk.Group == "" && gvk.Version == componentKind.Group) || (gvk.Group != "" && gvk.Group == componentKind.Group) {
					klog.V(packageInfoLogLevel).Info("Successfully found GVR ", gvr.String())

					var objlist *unstructured.UnstructuredList
					if _, ok := app.GetAnnotations()[corev1alpha1.AnnotationClusterScope]; ok {
						// retrieve all components, cluster wide
						objlist, err = r.explorer.DynamicMCClient.Resource(gvr).List(metav1.ListOptions{LabelSelector: labels.Set(app.Spec.Selector.MatchLabels).String()})
					} else {
						// retrieve only namespaced components
						objlist, err = r.explorer.DynamicMCClient.Resource(gvr).Namespace(obj.GetNamespace()).List(
							metav1.ListOptions{LabelSelector: labels.Set(app.Spec.Selector.MatchLabels).String()})
					}
					if err != nil {
						klog.Error("Failed to retrieve the list of components based on selector ")
						return err
					}
					if len(objlist.Items) == 0 {
						// we still want to create the deployables for the resources we find on managed cluster ,
						// even though some kinds defined in the app may not have corresponding (satisfying the selector)
						// resources on managed cluster
						klog.Info("Could not find a managed cluster resource for application component with kind ", componentKind.String())
					}
					appComponents[componentKind] = objlist
					break
				}
			}
		}
	}

	// process the components on managed cluster and creates deployables on hub for them
	for _, objlist := range appComponents {
		for _, item := range objlist.Items {
			klog.Info("Processing object ", item.GetName(), " in namespace ", item.GetNamespace(), " with kind ", item.GetKind())
			if err = deployable.SyncDeployable(&item, r.explorer); err != nil {
				klog.Error("Failed to sync deployable ", item.GetNamespace()+"/"+item.GetName(), " with error ", err)
			}
		}
	}
	return nil
}
