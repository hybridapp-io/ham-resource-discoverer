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

package utils

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
)

type Explorer struct {
	DynamicMCClient  dynamic.Interface
	DynamicHubClient dynamic.Interface
	Cluster          types.NamespacedName
	GVKGVRMap        map[schema.GroupVersionKind]schema.GroupVersionResource
}

const (
	packageDetailLogLevel = 5
)

var (
	resourcePredicate = discovery.SupportsAllVerbs{Verbs: []string{"create", "update", "delete", "list", "watch"}}
	explorer          *Explorer
)

func InitExplorer(hubconfig, mcconfig *rest.Config, cluster types.NamespacedName) (*Explorer, error) {
	var err error

	if explorer != nil {
		return explorer, nil
	}

	explorer = &Explorer{}
	explorer.DynamicMCClient, err = dynamic.NewForConfig(mcconfig)
	if err != nil {
		klog.Error("Failed to create dynamic client for explorer", err)
		return nil, err
	}

	explorer.DynamicHubClient, err = dynamic.NewForConfig(hubconfig)
	if err != nil {
		klog.Error("Failed to create dynamic client for explorer", err)
		return nil, err
	}

	explorer.Cluster = cluster
	resources, err := discovery.NewDiscoveryClientForConfigOrDie(mcconfig).ServerPreferredResources()
	if err != nil {
		klog.Error("Failed to discover all server resources, continuing with err:", err)
		return nil, err
	}

	filteredResources := discovery.FilteredBy(resourcePredicate, resources)

	klog.V(packageDetailLogLevel).Info("Discovered resources: ", filteredResources)

	explorer.GVKGVRMap = make(map[schema.GroupVersionKind]schema.GroupVersionResource)

	for _, rl := range filteredResources {
		buildGVKGVRMap(rl)
	}

	return explorer, nil

}

func buildGVKGVRMap(rl *metav1.APIResourceList) {
	for _, res := range rl.APIResources {
		gv, err := schema.ParseGroupVersion(rl.GroupVersion)
		if err != nil {
			klog.V(packageDetailLogLevel).Info("Skipping ", rl.GroupVersion, " with error:", err)
			continue
		}

		gvk := schema.GroupVersionKind{
			Kind:    res.Kind,
			Group:   gv.Group,
			Version: gv.Version,
		}
		gvr := schema.GroupVersionResource{
			Group:    gv.Group,
			Version:  gv.Version,
			Resource: res.Name,
		}

		explorer.GVKGVRMap[gvk] = gvr
	}
}
