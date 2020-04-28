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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	// AnnotationDiscovered indicates whether a resource has been created as a result of a discovery process
	AnnotationDiscovered = SchemeGroupVersion.Group + "/hybrid-discovered"

	//AnnotationClusterScope indicates whether discovery should look for resources cluster wide rather then in a specific namespace
	AnnotationClusterScope = SchemeGroupVersion.Group + "/hybrid-discover-clusterscoped"

	SourceObject = SchemeGroupVersion.Group + "/source-object"

	DeployerType = SchemeGroupVersion.Group + "/deployer-type"

	HostingDeployer = SchemeGroupVersion.Group + "/hosting-deployer"

	DeployerInCluster = SchemeGroupVersion.Group + "/deployer-in-cluster"
)

// DeployerSpec defines the desired state of Deployer
type DeployerSpec struct {
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	Type         string                  `json:"type"`
	OperatorRef  *corev1.ObjectReference `json:"operatorRef,omitempty"`
	Capabilities []rbacv1.PolicyRule     `json:"capabilities,omitempty"`
	ClusterScope bool                    `json:"clusterScope,omitempty"`
}

// DeployerStatus defines the observed state of Deployer
type DeployerStatus struct {
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Deployer is the Schema for the deployers API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=deployers,scope=Namespaced
type Deployer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec DeployerSpec `json:"spec,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DeployerList contains a list of Deployer
type DeployerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Deployer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Deployer{}, &DeployerList{})
}
