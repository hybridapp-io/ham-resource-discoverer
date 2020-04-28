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
)

var (
	managedClusterConfig *rest.Config
	hubClusterConfig     *rest.Config
)

func TestMain(m *testing.M) {
	// setup the managed cluster environment
	managedCluster := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "deploy", "crds"),
			filepath.Join("..", "..", "..", "hack", "test"),
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
			filepath.Join("..", "..", "..", "deploy", "crds"),
			filepath.Join("..", "..", "..", "hack", "test"),
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
	*ReconcileDeployable
	createCh chan interface{}
	updateCh chan interface{}
	deleteCh chan interface{}
}

func (ds DeployableSync) start() {
	ds.stop()
	// generic explorer
	ds.stopCh = make(chan struct{})
	handler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(new interface{}) {
			ds.syncCreateDeployable(new)
		},
		UpdateFunc: func(old, new interface{}) {
			ds.syncUpdateDeployable(old, new)
		},
		DeleteFunc: func(old interface{}) {
			ds.syncRemoveDeployable(old)
		},
	}

	ds.dynamicHubFactory.ForResource(ds.explorer.GVKGVRMap[deployableGVK]).Informer().AddEventHandler(handler)

	ds.stopCh = make(chan struct{})
	ds.dynamicHubFactory.Start(ds.stopCh)
}

func (ds DeployableSync) stop() {
	if ds.stopCh != nil {
		ds.dynamicHubFactory.WaitForCacheSync(ds.stopCh)
		close(ds.stopCh)
	}
	ds.stopCh = nil
}

func (ds DeployableSync) syncCreateDeployable(newObj interface{}) {
	ds.ReconcileDeployable.syncCreateDeployable(newObj)
	// non-blocking operation
	select {
	case ds.createCh <- newObj:
	default:
	}

}
func (ds DeployableSync) syncUpdateDeployable(oldObj, newObj interface{}) {
	ds.ReconcileDeployable.syncUpdateDeployable(oldObj, newObj)
	// non-blocking operation
	select {
	case ds.updateCh <- newObj:
	default:
	}

}
func (ds DeployableSync) syncRemoveDeployable(oldObj interface{}) {
	ds.ReconcileDeployable.syncRemoveDeployable(oldObj)
	// non-blocking operation
	select {
	case ds.deleteCh <- oldObj:
	default:
	}

}

func SetupDeployableSync(inner *ReconcileDeployable) ReconcileDeployableInterface {
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
