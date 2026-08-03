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

// GitHubOrganizationRole is the standard role granted in an organization.
// GitHub calls the admin role an organization owner in its user interface.
// +kubebuilder:validation:Enum=member;admin
type GitHubOrganizationRole string

const (
	GitHubOrganizationRoleMember GitHubOrganizationRole = "member"
	GitHubOrganizationRoleAdmin  GitHubOrganizationRole = "admin"
)

// GitHubOrganizationMemberSpec manages direct membership in the provider organization.
type GitHubOrganizationMemberSpec struct {
	// ProviderConfigRef references the cluster-scoped GitHubProviderConfig.
	// +kubebuilder:default=default
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="providerConfigRef is immutable"
	ProviderConfigRef string `json:"providerConfigRef,omitempty"`

	// Username is the GitHub login receiving organization membership.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="username is immutable"
	Username string `json:"username"`

	// Role is the organization role. The admin role grants organization ownership.
	Role GitHubOrganizationRole `json:"role"`

	// DeletionPolicy controls whether deleting this resource removes the user from the organization.
	// +kubebuilder:default=Orphan
	DeletionPolicy GitHubMembershipDeletionPolicy `json:"deletionPolicy,omitempty"`
}

// EffectiveProviderConfigRef returns the configured provider or the default provider.
func (s GitHubOrganizationMemberSpec) EffectiveProviderConfigRef() string {
	if s.ProviderConfigRef == "" {
		return DefaultGitHubProviderConfigName
	}
	return s.ProviderConfigRef
}

// EffectiveDeletionPolicy returns the configured policy or Orphan.
func (s GitHubOrganizationMemberSpec) EffectiveDeletionPolicy() GitHubMembershipDeletionPolicy {
	if s.DeletionPolicy == "" {
		return GitHubMembershipDeletionPolicyOrphan
	}
	return s.DeletionPolicy
}

// GitHubOrganizationMemberStatus defines the observed organization membership.
type GitHubOrganizationMemberStatus struct {
	// ProviderConfigRef is the provider used during reconciliation.
	// +optional
	ProviderConfigRef string `json:"providerConfigRef,omitempty"`

	// Organization is the resolved GitHub organization.
	// +optional
	Organization string `json:"organization,omitempty"`

	// Username is the reconciled GitHub login.
	// +optional
	Username string `json:"username,omitempty"`

	// Role is the observed organization role.
	// +optional
	Role GitHubOrganizationRole `json:"role,omitempty"`

	// State is the GitHub membership state, normally active or pending.
	// +optional
	State string `json:"state,omitempty"`

	// InvitationPending indicates that the user still needs to accept the invitation.
	// +optional
	InvitationPending bool `json:"invitationPending,omitempty"`

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
// +kubebuilder:resource:shortName=ghorgmember
// +kubebuilder:printcolumn:name="Organization",type=string,JSONPath=`.status.organization`
// +kubebuilder:printcolumn:name="Username",type=string,JSONPath=`.spec.username`
// +kubebuilder:printcolumn:name="Role",type=string,JSONPath=`.status.role`
// +kubebuilder:printcolumn:name="Pending",type=boolean,JSONPath=`.status.invitationPending`
// +kubebuilder:printcolumn:name="Deletion Policy",type=string,JSONPath=`.spec.deletionPolicy`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// GitHubOrganizationMember manages one direct organization member or owner.
type GitHubOrganizationMember struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitHubOrganizationMemberSpec   `json:"spec,omitempty"`
	Status GitHubOrganizationMemberStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GitHubOrganizationMemberList contains a list of GitHubOrganizationMember resources.
type GitHubOrganizationMemberList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitHubOrganizationMember `json:"items"`
}
