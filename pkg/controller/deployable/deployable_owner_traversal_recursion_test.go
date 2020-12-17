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
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	//	. "github.com/onsi/gomega"
)

var (
	mcPod = &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testPod",
			Namespace: mcName,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "Deployment",
					Name: "testDeployment",
				},
			},
		},
	}
)

func TestOwnerTraversal(t *testing.T) {

	obj, err := findRootResource(mcPod)

	// g.Expect(annotations[dplv1.AnnotationHosting]).To(Equal(dpl.Namespace + "/" + dpl.Name))
	// g.Expect(annotations[subv1.AnnotationHosting]).To(Equal("/"))
	// g.Expect(annotations[subv1.AnnotationSyncSource]).To(Equal("subnsdpl-/"))
}
