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
	"log"
	"os"
	"path/filepath"
	"sync"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/onsi/gomega"

	apis "github.com/hybridapp-io/ham-resource-discoverer/pkg/apis"
)

const (
	clusterNameOnHub      = "reave"
	clusterNamespaceOnHub = "reave"
)

var (
	managedClusterConfig *rest.Config
	hubClusterConfig     *rest.Config

	// managed cluster namespace on hub
	clusterOnHub = types.NamespacedName{
		Name:      clusterNameOnHub,
		Namespace: clusterNamespaceOnHub,
	}

	nsOnHub = v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterNamespaceOnHub,
		},
	}
)

func TestMain(m *testing.M) {
	// setup the managed cluster environment
	managedCluster := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "deploy", "crds"),
			filepath.Join("..", "..", "..", "hack", "test"),
		},
	}

	// add eployer-operator scheme
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

type HubClient struct {
	client.Client
	createCh chan runtime.Object
	deleteCh chan runtime.Object
	updateCh chan runtime.Object
}

func (hubClient HubClient) Create(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
	err := hubClient.Client.Create(ctx, obj, opts...)
	// non-blocking operation
	select {
	case hubClient.createCh <- obj:
	default:
	}
	return err
}

func (hubClient HubClient) Delete(ctx context.Context, obj runtime.Object, opts ...client.DeleteOption) error {
	err := hubClient.Client.Delete(ctx, obj, opts...)
	// non-blocking operation
	select {
	case hubClient.deleteCh <- obj:
	default:
	}
	return err
}

func (hubClient HubClient) Update(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
	err := hubClient.Client.Update(ctx, obj, opts...)
	// non-blocking operation
	select {
	case hubClient.updateCh <- obj:
	default:
	}
	return err
}

func SetupHubClient(innerClient client.Client) client.Client {
	cCh := make(chan runtime.Object, 5)
	dCh := make(chan runtime.Object, 5)
	uCh := make(chan runtime.Object, 5)

	hubClient := HubClient{
		Client:   innerClient,
		createCh: cCh,
		deleteCh: dCh,
		updateCh: uCh,
	}
	return hubClient
}

func SetupTestReconcile(inner reconcile.Reconciler) (reconcile.Reconciler, chan reconcile.Request) {
	requests := make(chan reconcile.Request)
	fn := reconcile.Func(func(req reconcile.Request) (reconcile.Result, error) {
		result, err := inner.Reconcile(req)
		requests <- req
		return result, err
	})

	return fn, requests
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
