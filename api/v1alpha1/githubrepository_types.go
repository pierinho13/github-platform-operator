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

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RepositoryVisibility defines the visibility supported by GitHub repositories.
// +kubebuilder:validation:Enum=public;private
type RepositoryVisibility string

const (
	// RepositoryVisibilityPublic makes the repository publicly accessible.
	RepositoryVisibilityPublic RepositoryVisibility = "public"

	// RepositoryVisibilityPrivate restricts access to authorized users.
	RepositoryVisibilityPrivate RepositoryVisibility = "private"
)

// GitHubRepositorySpec defines the desired state of GitHubRepository.
type GitHubRepositorySpec struct {
	// Organization is the GitHub organization where the repository will be created.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="organization is immutable"
	Organization string `json:"organization"`

	// Name is the name of the GitHub repository.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Visibility determines whether the repository is public or private.
	// +kubebuilder:default=private
	Visibility RepositoryVisibility `json:"visibility,omitempty"`
}

// GitHubRepositoryStatus defines the observed state of GitHubRepository.
type GitHubRepositoryStatus struct {
	// RepositoryID is the numeric identifier assigned by GitHub.
	// +optional
	RepositoryID int64 `json:"repositoryId,omitempty"`

	// URL is the HTML URL of the GitHub repository.
	// +optional
	URL string `json:"url,omitempty"`

	// Visibility is the repository visibility last observed in GitHub.
	// +optional
	Visibility RepositoryVisibility `json:"visibility,omitempty"`

	// ObservedGeneration is the GitHubRepository generation most recently reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions describe the current reconciliation state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ghrepo
// +kubebuilder:printcolumn:name="Organization",type=string,JSONPath=`.spec.organization`
// +kubebuilder:printcolumn:name="Repository",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="Visibility",type=string,JSONPath=`.status.visibility`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// GitHubRepository is the Schema for the githubrepositories API.
type GitHubRepository struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitHubRepositorySpec   `json:"spec,omitempty"`
	Status GitHubRepositoryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GitHubRepositoryList contains a list of GitHubRepository.
type GitHubRepositoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitHubRepository `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GitHubRepository{}, &GitHubRepositoryList{})
}
