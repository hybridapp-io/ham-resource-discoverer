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
package ocm

import (
	"github.com/hybridapp-io/ham-resource-discoverer/pkg/controller/application"
	"github.com/hybridapp-io/ham-resource-discoverer/pkg/synchronizer"
	"github.com/hybridapp-io/ham-resource-discoverer/pkg/synchronizer/ocm"
	"github.com/hybridapp-io/ham-resource-discoverer/pkg/utils"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Add creates a newObj Deployable Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, hubconfig *rest.Config, cluster types.NamespacedName,
	hubSynchronizer synchronizer.HubSynchronizerInterface) error {
	explorer, err := utils.InitExplorer(hubconfig, mgr.GetConfig(), cluster)
	if err != nil {
		klog.Error("Failed to initialize the explorer")
		return err
	}

	hubSynchronizer.(*ocm.HubSynchronizer).Explorer = explorer

	reconciler, err := application.NewReconciler(mgr, hubconfig, cluster, explorer, hubSynchronizer)
	if err != nil {
		klog.Error("Failed to create the application reconciler ", err)
		return err
	}
	reconciler.Start()
	return nil
}
