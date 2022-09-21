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

package apis

import (
	sigappapis "github.com/kubernetes-sigs/application/pkg/apis"
	"k8s.io/apimachinery/pkg/runtime"

	manifestwork "github.com/open-cluster-management/api/work/v1"
	dplapis "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis"
)

// AddToSchemes may be used to add all resources defined in the project to a Scheme
var AddToSchemes runtime.SchemeBuilder

// AddToScheme adds all Resources to the Scheme
func AddToScheme(s *runtime.Scheme) error {
	err := dplapis.AddToScheme(s)
	if err != nil {
		return err
	}

	err = sigappapis.AddToScheme(s)
	if err != nil {
		return err
	}

	err = manifestwork.AddToScheme(s)
	if err != nil {
		return err
	}

	return AddToSchemes.AddToScheme(s)
}
