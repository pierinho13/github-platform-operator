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

// GitHubTeamMembershipRole is the role granted inside a team.
// +kubebuilder:validation:Enum=member;maintainer
type GitHubTeamMembershipRole string

const (
	GitHubTeamMembershipRoleMember     GitHubTeamMembershipRole = "member"
	GitHubTeamMembershipRoleMaintainer GitHubTeamMembershipRole = "maintainer"
)

// GitHubMembershipDeletionPolicy controls whether remote membership is revoked.
// +kubebuilder:validation:Enum=Orphan;Revoke
type GitHubMembershipDeletionPolicy string

const (
	GitHubMembershipDeletionPolicyOrphan GitHubMembershipDeletionPolicy = "Orphan"
	GitHubMembershipDeletionPolicyRevoke GitHubMembershipDeletionPolicy = "Revoke"
)

// GitHubTeamMembershipSpec assigns a user to a managed GitHub team.
type GitHubTeamMembershipSpec struct {
	// TeamRef references a GitHubTeam in the same namespace.
	TeamRef GitHubTeamReference `json:"teamRef"`

	// Username is the GitHub login assigned to the team.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="username is immutable"
	Username string `json:"username"`

	// Role is the user's role in the team.
	Role GitHubTeamMembershipRole `json:"role"`

	// DeletionPolicy controls whether deleting this resource removes team membership.
	// +kubebuilder:default=Orphan
	DeletionPolicy GitHubMembershipDeletionPolicy `json:"deletionPolicy,omitempty"`
}

// EffectiveDeletionPolicy returns the configured policy or Orphan.
func (s GitHubTeamMembershipSpec) EffectiveDeletionPolicy() GitHubMembershipDeletionPolicy {
	if s.DeletionPolicy == "" {
		return GitHubMembershipDeletionPolicyOrphan
	}
	return s.DeletionPolicy
}

// GitHubTeamMembershipStatus defines the observed team membership.
type GitHubTeamMembershipStatus struct {
	// ProviderConfigRef is the provider used during reconciliation.
	// +optional
	ProviderConfigRef string `json:"providerConfigRef,omitempty"`

	// Organization is the resolved GitHub organization.
	// +optional
	Organization string `json:"organization,omitempty"`

	// Team is the referenced Kubernetes GitHubTeam name.
	// +optional
	Team string `json:"team,omitempty"`

	// TeamSlug is the reconciled GitHub team slug.
	// +optional
	TeamSlug string `json:"teamSlug,omitempty"`

	// Username is the reconciled GitHub login.
	// +optional
	Username string `json:"username,omitempty"`

	// Role is the observed team role.
	// +optional
	Role GitHubTeamMembershipRole `json:"role,omitempty"`

	// State is the GitHub membership state, normally active or pending.
	// +optional
	State string `json:"state,omitempty"`

	// InvitationPending indicates that the user has not completed organization membership.
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
// +kubebuilder:resource:shortName=ghteammember
// +kubebuilder:printcolumn:name="Team",type=string,JSONPath=`.status.teamSlug`
// +kubebuilder:printcolumn:name="Username",type=string,JSONPath=`.spec.username`
// +kubebuilder:printcolumn:name="Role",type=string,JSONPath=`.status.role`
// +kubebuilder:printcolumn:name="Pending",type=boolean,JSONPath=`.status.invitationPending`
// +kubebuilder:printcolumn:name="Deletion Policy",type=string,JSONPath=`.spec.deletionPolicy`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// GitHubTeamMembership manages one user's membership in a GitHub team.
type GitHubTeamMembership struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitHubTeamMembershipSpec   `json:"spec,omitempty"`
	Status GitHubTeamMembershipStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GitHubTeamMembershipList contains a list of GitHubTeamMembership resources.
type GitHubTeamMembershipList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitHubTeamMembership `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GitHubTeamMembership{}, &GitHubTeamMembershipList{})
}
