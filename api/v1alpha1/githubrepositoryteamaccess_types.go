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

// GitHubRepositoryTeamAccessSpec defines a team assignment to a repository.
type GitHubRepositoryTeamAccessSpec struct {
	// RepositoryRef references a GitHubRepository in the same namespace.
	RepositoryRef GitHubRepositoryReference `json:"repositoryRef"`

	// TeamSlug is the URL slug of an existing GitHub organization team.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="teamSlug is immutable"
	TeamSlug string `json:"teamSlug"`

	// Permission is the repository role granted to the team.
	Permission RepositoryPermission `json:"permission"`

	// DeletionPolicy controls whether deleting this resource revokes the team access.
	// +kubebuilder:default=Orphan
	DeletionPolicy RepositoryAccessDeletionPolicy `json:"deletionPolicy,omitempty"`
}

// EffectiveDeletionPolicy returns the configured policy or the safe Orphan default.
func (s GitHubRepositoryTeamAccessSpec) EffectiveDeletionPolicy() RepositoryAccessDeletionPolicy {
	return EffectiveRepositoryAccessDeletionPolicy(s.DeletionPolicy)
}

// GitHubRepositoryTeamAccessStatus defines the observed team assignment.
type GitHubRepositoryTeamAccessStatus struct {
	// ProviderConfigRef is the provider used during reconciliation.
	// +optional
	ProviderConfigRef string `json:"providerConfigRef,omitempty"`

	// Organization is the resolved GitHub organization.
	// +optional
	Organization string `json:"organization,omitempty"`

	// Repository is the resolved GitHub repository name.
	// +optional
	Repository string `json:"repository,omitempty"`

	// TeamSlug is the reconciled team slug.
	// +optional
	TeamSlug string `json:"teamSlug,omitempty"`

	// Permission is the permission currently observed in GitHub.
	// +optional
	Permission RepositoryPermission `json:"permission,omitempty"`

	// ObservedGeneration is the generation most recently reconciled.
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
// +kubebuilder:resource:shortName=ghteamaccess
// +kubebuilder:printcolumn:name="Repository",type=string,JSONPath=`.status.repository`
// +kubebuilder:printcolumn:name="Team",type=string,JSONPath=`.spec.teamSlug`
// +kubebuilder:printcolumn:name="Permission",type=string,JSONPath=`.status.permission`
// +kubebuilder:printcolumn:name="Deletion Policy",type=string,JSONPath=`.spec.deletionPolicy`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// GitHubRepositoryTeamAccess assigns an existing GitHub team to a repository.
type GitHubRepositoryTeamAccess struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitHubRepositoryTeamAccessSpec   `json:"spec,omitempty"`
	Status GitHubRepositoryTeamAccessStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GitHubRepositoryTeamAccessList contains a list of GitHubRepositoryTeamAccess.
type GitHubRepositoryTeamAccessList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitHubRepositoryTeamAccess `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GitHubRepositoryTeamAccess{}, &GitHubRepositoryTeamAccessList{})
}
