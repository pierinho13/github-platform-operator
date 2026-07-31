/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// EnvironmentDeletionPolicy defines what happens to the GitHub environment
// when its Kubernetes resource is deleted.
// +kubebuilder:validation:Enum=Orphan;Delete
type EnvironmentDeletionPolicy string

const (
	EnvironmentDeletionPolicyOrphan EnvironmentDeletionPolicy = "Orphan"
	EnvironmentDeletionPolicyDelete EnvironmentDeletionPolicy = "Delete"
)

// GitHubEnvironmentSpec defines a basic repository deployment environment.
type GitHubEnvironmentSpec struct {
	// RepositoryRef references a GitHubRepository in the same namespace.
	RepositoryRef GitHubRepositoryReference `json:"repositoryRef"`

	// Name is the GitHub environment name.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// DeletionPolicy controls whether deleting this resource deletes the environment.
	// +kubebuilder:default=Orphan
	DeletionPolicy EnvironmentDeletionPolicy `json:"deletionPolicy,omitempty"`
}

// EffectiveDeletionPolicy returns the configured policy or the safe Orphan default.
func (s GitHubEnvironmentSpec) EffectiveDeletionPolicy() EnvironmentDeletionPolicy {
	if s.DeletionPolicy == "" {
		return EnvironmentDeletionPolicyOrphan
	}
	return s.DeletionPolicy
}

// GitHubEnvironmentStatus defines the observed environment state.
type GitHubEnvironmentStatus struct {
	// ProviderConfigRef is the resolved provider.
	// +optional
	ProviderConfigRef string `json:"providerConfigRef,omitempty"`
	// Organization is the resolved organization.
	// +optional
	Organization string `json:"organization,omitempty"`
	// Repository is the resolved repository name.
	// +optional
	Repository string `json:"repository,omitempty"`
	// Environment is the observed environment name.
	// +optional
	Environment string `json:"environment,omitempty"`
	// EnvironmentID is the GitHub environment identifier.
	// +optional
	EnvironmentID int64 `json:"environmentId,omitempty"`
	// ObservedGeneration is the generation most recently reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Conditions describe the reconciliation state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ghenv
// +kubebuilder:printcolumn:name="Repository",type=string,JSONPath=`.status.repository`
// +kubebuilder:printcolumn:name="Environment",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="Deletion Policy",type=string,JSONPath=`.spec.deletionPolicy`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// GitHubEnvironment manages a basic GitHub deployment environment.
type GitHubEnvironment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitHubEnvironmentSpec   `json:"spec,omitempty"`
	Status GitHubEnvironmentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GitHubEnvironmentList contains a list of GitHubEnvironment.
type GitHubEnvironmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitHubEnvironment `json:"items"`
}
