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
	"log"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/onsi/gomega"

	apis "github.com/hybridapp-io/ham-resource-discoverer/pkg/apis"
	"github.com/hybridapp-io/ham-resource-discoverer/pkg/controller/deployable"
)

var (
	managedClusterConfig *rest.Config
	hubClusterConfig     *rest.Config
)

func TestMain(m *testing.M) {
	// setup the managed cluster environment
	managedCluster := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "..", "deploy", "crds"),
			filepath.Join("..", "..", "..", "..", "hack", "test"),
		},
	}

	// add resource-discoverer scheme
	err := apis.AddToScheme(scheme.Scheme)
	if err != nil {
		log.Fatal(err)
	}

	if managedClusterConfig, err = managedCluster.Start(); err != nil {
		log.Fatal(err)
	}

	// setup the hub cluster environment
	hubCluster := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "..", "deploy", "crds"),
			filepath.Join("..", "..", "..", "..", "hack", "test"),
		},
	}

	if hubClusterConfig, err = hubCluster.Start(); err != nil {
		log.Fatal(err)
	}

	code := m.Run()

	if err = managedCluster.Stop(); err != nil {
		log.Fatal(err)
	}
	if err = hubCluster.Stop(); err != nil {
		log.Fatal(err)
	}
	os.Exit(code)
}

const waitgroupDelta = 1

type DeployableSync struct {
	*deployable.ReconcileDeployable
	createCh chan interface{}
	updateCh chan interface{}
	deleteCh chan interface{}
}

func (ds DeployableSync) Start() {
	ds.Stop()
	// generic explorer
	ds.StopCh = make(chan struct{})
	handler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(new interface{}) {
			ds.SyncCreateDeployable(new)
		},
		UpdateFunc: func(old, new interface{}) {
			ds.SyncUpdateDeployable(old, new)
		},
		DeleteFunc: func(old interface{}) {
			ds.SyncRemoveDeployable(old)
		},
	}

	ds.DynamicHubFactory.ForResource(deployableGVR).Informer().AddEventHandler(handler)

	ds.StopCh = make(chan struct{})
	ds.DynamicHubFactory.Start(ds.StopCh)
}

func (ds DeployableSync) Stop() {
	if ds.StopCh != nil {
		ds.DynamicHubFactory.WaitForCacheSync(ds.StopCh)
		close(ds.StopCh)
	}
	ds.StopCh = nil
}

func (ds DeployableSync) SyncCreateDeployable(newObj interface{}) {
	ds.ReconcileDeployable.SyncCreateDeployable(newObj)
	// non-blocking operation
	select {
	case ds.createCh <- newObj:
	default:
	}

}
func (ds DeployableSync) SyncUpdateDeployable(oldObj, newObj interface{}) {
	ds.ReconcileDeployable.SyncUpdateDeployable(oldObj, newObj)
	// non-blocking operation
	select {
	case ds.updateCh <- newObj:
	default:
	}

}
func (ds DeployableSync) SyncRemoveDeployable(oldObj interface{}) {
	ds.ReconcileDeployable.SyncRemoveDeployable(oldObj)
	// non-blocking operation
	select {
	case ds.deleteCh <- oldObj:
	default:
	}

}

func SetupDeployableSync(inner *deployable.ReconcileDeployable) deployable.ReconcileDeployableInterface {
	cCh := make(chan interface{}, 5)
	uCh := make(chan interface{}, 5)
	dCh := make(chan interface{}, 5)

	dplSync := DeployableSync{
		ReconcileDeployable: inner,
		createCh:            cCh,
		updateCh:            uCh,
		deleteCh:            dCh,
	}
	return dplSync
}

// StartTestManager adds recFn
func StartTestManager(mgr manager.Manager, g *gomega.GomegaWithT) (chan struct{}, *sync.WaitGroup) {
	stop := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(waitgroupDelta)

	go func() {
		defer wg.Done()
		g.Expect(mgr.Start(stop)).NotTo(gomega.HaveOccurred())
	}()

	return stop, wg
}
