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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const (
	//IsDefaultDeployer defines annotation used to indicate if a deployer is considered default for a cluster
	IsDefaultDeployer = "app.cp4mcm.ibm.com/is-default-deployer"
)

// DeployerSpecDescriptor defines the deployer structure
type DeployerSpecDescriptor struct {
	// NamespacedName of deployer for key
	Key  string       `json:"key"`
	Spec DeployerSpec `json:"spec"`
}

// DeployerSetSpec defines the desired state of DeployerSet
type DeployerSetSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	DefaultDeployer string                   `json:"defaultDeployer,omitempty"`
	Deployers       []DeployerSpecDescriptor `json:"deployers,omitempty"`
}

// DeployerStatusDescriptor defines the deployer status
type DeployerStatusDescriptor struct {
	// NamespacedName of deployer for key
	Status DeployerStatus `json:"status"`
	Key    string         `json:"key"`
}

// DeployerSetStatus defines the observed state of DeployerSet
type DeployerSetStatus struct {
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html

	Deployers []DeployerStatusDescriptor `json:"deployers,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DeployerSet is the Schema for the deployersets API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=deployersets,scope=Namespaced
type DeployerSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeployerSetSpec   `json:"spec,omitempty"`
	Status DeployerSetStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DeployerSetList contains a list of DeployerSet
type DeployerSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DeployerSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DeployerSet{}, &DeployerSetList{})
}
