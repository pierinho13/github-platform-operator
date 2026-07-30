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

// GitHubRepositoryCollaboratorSpec defines direct user access to a repository.
type GitHubRepositoryCollaboratorSpec struct {
	// RepositoryRef references a GitHubRepository in the same namespace.
	RepositoryRef GitHubRepositoryReference `json:"repositoryRef"`

	// Username is the GitHub login receiving direct repository access.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="username is immutable"
	Username string `json:"username"`

	// Permission is the repository role granted directly to the user.
	Permission RepositoryPermission `json:"permission"`

	// DeletionPolicy controls whether deleting this resource revokes direct access.
	// +kubebuilder:default=Orphan
	DeletionPolicy RepositoryAccessDeletionPolicy `json:"deletionPolicy,omitempty"`
}

// EffectiveDeletionPolicy returns the configured policy or the safe Orphan default.
func (s GitHubRepositoryCollaboratorSpec) EffectiveDeletionPolicy() RepositoryAccessDeletionPolicy {
	return EffectiveRepositoryAccessDeletionPolicy(s.DeletionPolicy)
}

// GitHubRepositoryCollaboratorStatus defines the observed direct user access.
type GitHubRepositoryCollaboratorStatus struct {
	// ProviderConfigRef is the provider used during reconciliation.
	// +optional
	ProviderConfigRef string `json:"providerConfigRef,omitempty"`

	// Organization is the resolved GitHub organization.
	// +optional
	Organization string `json:"organization,omitempty"`

	// Repository is the resolved GitHub repository name.
	// +optional
	Repository string `json:"repository,omitempty"`

	// Username is the reconciled GitHub login.
	// +optional
	Username string `json:"username,omitempty"`

	// Permission is the permission currently observed in GitHub.
	// +optional
	Permission RepositoryPermission `json:"permission,omitempty"`

	// InvitationPending indicates that the user still needs to accept the invitation.
	// +optional
	InvitationPending bool `json:"invitationPending,omitempty"`

	// InvitationID is the pending GitHub repository invitation identifier.
	// +optional
	InvitationID int64 `json:"invitationId,omitempty"`

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
// +kubebuilder:resource:shortName=ghcollab
// +kubebuilder:printcolumn:name="Repository",type=string,JSONPath=`.status.repository`
// +kubebuilder:printcolumn:name="Username",type=string,JSONPath=`.spec.username`
// +kubebuilder:printcolumn:name="Permission",type=string,JSONPath=`.status.permission`
// +kubebuilder:printcolumn:name="Pending",type=boolean,JSONPath=`.status.invitationPending`
// +kubebuilder:printcolumn:name="Deletion Policy",type=string,JSONPath=`.spec.deletionPolicy`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// GitHubRepositoryCollaborator grants a user direct access to a repository.
type GitHubRepositoryCollaborator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitHubRepositoryCollaboratorSpec   `json:"spec,omitempty"`
	Status GitHubRepositoryCollaboratorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GitHubRepositoryCollaboratorList contains a list of GitHubRepositoryCollaborator.
type GitHubRepositoryCollaboratorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitHubRepositoryCollaborator `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GitHubRepositoryCollaborator{}, &GitHubRepositoryCollaboratorList{})
}
