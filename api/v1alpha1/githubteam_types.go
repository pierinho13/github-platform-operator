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

// GitHubTeamPrivacy controls team visibility inside the organization.
// +kubebuilder:validation:Enum=closed;secret
type GitHubTeamPrivacy string

const (
	GitHubTeamPrivacyClosed GitHubTeamPrivacy = "closed"
	GitHubTeamPrivacySecret GitHubTeamPrivacy = "secret"
)

// GitHubTeamDeletionPolicy controls remote team cleanup.
// +kubebuilder:validation:Enum=Orphan;Delete
type GitHubTeamDeletionPolicy string

const (
	GitHubTeamDeletionPolicyOrphan GitHubTeamDeletionPolicy = "Orphan"
	GitHubTeamDeletionPolicyDelete GitHubTeamDeletionPolicy = "Delete"
)

// GitHubTeamReference references a GitHubTeam in the same namespace.
type GitHubTeamReference struct {
	// Name is the Kubernetes GitHubTeam resource name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// GitHubTeamSpec defines an organization team.
type GitHubTeamSpec struct {
	// ProviderConfigRef references the cluster-scoped GitHubProviderConfig.
	// +kubebuilder:default=default
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="providerConfigRef is immutable"
	ProviderConfigRef string `json:"providerConfigRef,omitempty"`

	// Name is the GitHub team display name. It is immutable to keep references stable.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Description is the team description. When omitted, an adopted team's existing
	// description is preserved. Set it to an empty string to clear it.
	// +optional
	Description *string `json:"description,omitempty"`

	// Privacy controls whether the team is visible to all organization members or
	// only to its members. When omitted, new teams are created as closed and adopted
	// teams preserve their current privacy.
	// +optional
	Privacy *GitHubTeamPrivacy `json:"privacy,omitempty"`

	// DeletionPolicy controls whether deleting this resource deletes the GitHub team.
	// +kubebuilder:default=Orphan
	DeletionPolicy GitHubTeamDeletionPolicy `json:"deletionPolicy,omitempty"`
}

// EffectiveProviderConfigRef returns the configured provider or the default provider.
func (s GitHubTeamSpec) EffectiveProviderConfigRef() string {
	if s.ProviderConfigRef == "" {
		return DefaultGitHubProviderConfigName
	}
	return s.ProviderConfigRef
}

// EffectivePrivacyForCreation returns the safe visible-to-organization default.
func (s GitHubTeamSpec) EffectivePrivacyForCreation() GitHubTeamPrivacy {
	if s.Privacy == nil {
		return GitHubTeamPrivacyClosed
	}
	return *s.Privacy
}

// EffectiveDeletionPolicy returns the configured policy or Orphan.
func (s GitHubTeamSpec) EffectiveDeletionPolicy() GitHubTeamDeletionPolicy {
	if s.DeletionPolicy == "" {
		return GitHubTeamDeletionPolicyOrphan
	}
	return s.DeletionPolicy
}

// GitHubTeamStatus defines the observed team state.
type GitHubTeamStatus struct {
	// ProviderConfigRef is the provider used during reconciliation.
	// +optional
	ProviderConfigRef string `json:"providerConfigRef,omitempty"`

	// Organization is the resolved GitHub organization.
	// +optional
	Organization string `json:"organization,omitempty"`

	// TeamID is the numeric GitHub team identifier.
	// +optional
	TeamID int64 `json:"teamId,omitempty"`

	// Name is the observed GitHub team name.
	// +optional
	Name string `json:"name,omitempty"`

	// Slug is the observed GitHub team URL slug.
	// +optional
	Slug string `json:"slug,omitempty"`

	// Description is the observed team description.
	// +optional
	Description string `json:"description,omitempty"`

	// Privacy is the observed team privacy.
	// +optional
	Privacy GitHubTeamPrivacy `json:"privacy,omitempty"`

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
// +kubebuilder:resource:shortName=ghteam
// +kubebuilder:printcolumn:name="Organization",type=string,JSONPath=`.status.organization`
// +kubebuilder:printcolumn:name="Team",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="Slug",type=string,JSONPath=`.status.slug`
// +kubebuilder:printcolumn:name="Privacy",type=string,JSONPath=`.status.privacy`
// +kubebuilder:printcolumn:name="Deletion Policy",type=string,JSONPath=`.spec.deletionPolicy`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// GitHubTeam creates or adopts an organization team.
type GitHubTeam struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitHubTeamSpec   `json:"spec,omitempty"`
	Status GitHubTeamStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GitHubTeamList contains a list of GitHubTeam resources.
type GitHubTeamList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitHubTeam `json:"items"`
}
