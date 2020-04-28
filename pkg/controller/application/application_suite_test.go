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

type ApplicationSync struct {
	*ReconcileApplication
	createCh chan interface{}
	updateCh chan interface{}
	deleteCh chan interface{}
}

func (as ApplicationSync) start() {
	as.stop()
	// generic explorer
	as.stopCh = make(chan struct{})
	handler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(newObj interface{}) {
			as.syncCreateApplication(newObj)
		},
		UpdateFunc: func(old, newObj interface{}) {
			as.syncUpdateApplication(old, newObj)
		},
		DeleteFunc: func(old interface{}) {
			as.syncRemoveApplication(old)
		},
	}

	as.dynamicMCFactory.ForResource(as.explorer.GVKGVRMap[applicationGVK]).Informer().AddEventHandler(handler)

	as.stopCh = make(chan struct{})
	as.dynamicMCFactory.Start(as.stopCh)
}

func (as ApplicationSync) stop() {
	if as.stopCh != nil {
		as.dynamicMCFactory.WaitForCacheSync(as.stopCh)
		close(as.stopCh)
	}
	as.stopCh = nil
}

func (as ApplicationSync) syncCreateApplication(obj interface{}) {
	as.ReconcileApplication.syncCreateApplication(obj)
	// non-blocking operation
	select {
	case as.createCh <- obj:
	default:
	}

}
func (as ApplicationSync) syncUpdateApplication(old, newObj interface{}) {
	as.ReconcileApplication.syncUpdateApplication(old, newObj)
	// non-blocking operation
	select {
	case as.updateCh <- newObj:
	default:
	}

}
func (as ApplicationSync) syncRemoveApplication(old interface{}) {
	as.ReconcileApplication.syncRemoveApplication(old)
	// non-blocking operation
	select {
	case as.deleteCh <- old:
	default:
	}

}

func SetupApplicationSync(inner *ReconcileApplication) ReconcileApplicationInterface {
	cCh := make(chan interface{}, 5)
	uCh := make(chan interface{}, 5)
	dCh := make(chan interface{}, 5)

	appSync := ApplicationSync{
		ReconcileApplication: inner,
		createCh:             cCh,
		updateCh:             uCh,
		deleteCh:             dCh,
	}
	return appSync
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
