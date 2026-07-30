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

// ActionsTargetScope identifies where an Actions secret or variable is stored.
type ActionsTargetScope string

const (
	ActionsTargetScopeRepository   ActionsTargetScope = "repository"
	ActionsTargetScopeEnvironment  ActionsTargetScope = "environment"
	ActionsTargetScopeOrganization ActionsTargetScope = "organization"
)

// ActionsResourceDeletionPolicy defines what happens to an Actions secret or
// variable when its Kubernetes resource is deleted.
// +kubebuilder:validation:Enum=Orphan;Revoke
type ActionsResourceDeletionPolicy string

const (
	ActionsResourceDeletionPolicyOrphan ActionsResourceDeletionPolicy = "Orphan"
	ActionsResourceDeletionPolicyRevoke ActionsResourceDeletionPolicy = "Revoke"
)

// EffectiveActionsResourceDeletionPolicy returns the configured policy or the
// safe Orphan default.
func EffectiveActionsResourceDeletionPolicy(
	policy ActionsResourceDeletionPolicy,
) ActionsResourceDeletionPolicy {
	if policy == "" {
		return ActionsResourceDeletionPolicyOrphan
	}
	return policy
}

// OrganizationActionsVisibility defines which repositories can consume an
// organization Actions secret or variable.
// +kubebuilder:validation:Enum=all;private;selected
type OrganizationActionsVisibility string

const (
	OrganizationActionsVisibilityAll      OrganizationActionsVisibility = "all"
	OrganizationActionsVisibilityPrivate  OrganizationActionsVisibility = "private"
	OrganizationActionsVisibilitySelected OrganizationActionsVisibility = "selected"
)

// GitHubEnvironmentReference references a GitHubEnvironment in the same namespace.
type GitHubEnvironmentReference struct {
	// Name is the Kubernetes metadata.name of the GitHubEnvironment.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="environmentRef.name is immutable"
	Name string `json:"name"`
}

// LocalSecretKeyReference selects a key from a Kubernetes Secret in the same namespace.
type LocalSecretKeyReference struct {
	// Name is the Secret name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Key is the Secret data key.
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}

// ActionsValueSource defines where the value is read from.
type ActionsValueSource struct {
	// SecretKeyRef selects a value from a Kubernetes Secret in the same namespace.
	SecretKeyRef LocalSecretKeyReference `json:"secretKeyRef"`
}

// GitHubOrganizationActionsTarget selects an organization through a provider.
type GitHubOrganizationActionsTarget struct {
	// ProviderConfigRef references the cluster-scoped GitHubProviderConfig.
	// +kubebuilder:default=default
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="providerConfigRef is immutable"
	ProviderConfigRef string `json:"providerConfigRef,omitempty"`

	// Visibility controls which repositories can consume the organization value.
	// +kubebuilder:default=private
	Visibility OrganizationActionsVisibility `json:"visibility,omitempty"`

	// SelectedRepositoryRefs contains GitHubRepository resources in the same
	// namespace. It is used only when visibility is selected.
	// +optional
	// +kubebuilder:validation:MaxItems=100
	// +listType=map
	// +listMapKey=name
	SelectedRepositoryRefs []GitHubRepositoryReference `json:"selectedRepositoryRefs,omitempty"`
}

// EffectiveProviderConfigRef returns the configured provider or the default provider.
func (t GitHubOrganizationActionsTarget) EffectiveProviderConfigRef() string {
	if t.ProviderConfigRef == "" {
		return DefaultGitHubProviderConfigName
	}
	return t.ProviderConfigRef
}

// EffectiveVisibility returns the configured visibility or the safe private default.
func (t GitHubOrganizationActionsTarget) EffectiveVisibility() OrganizationActionsVisibility {
	if t.Visibility == "" {
		return OrganizationActionsVisibilityPrivate
	}
	return t.Visibility
}

// GitHubActionsTarget selects exactly one repository, environment, or organization.
// +kubebuilder:validation:XValidation:rule="((has(self.repositoryRef) ? 1 : 0) + (has(self.environmentRef) ? 1 : 0) + (has(self.organization) ? 1 : 0)) == 1",message="exactly one target must be configured"
type GitHubActionsTarget struct {
	// RepositoryRef targets a GitHubRepository in the same namespace.
	// +optional
	RepositoryRef *GitHubRepositoryReference `json:"repositoryRef,omitempty"`

	// EnvironmentRef targets a GitHubEnvironment in the same namespace.
	// +optional
	EnvironmentRef *GitHubEnvironmentReference `json:"environmentRef,omitempty"`

	// Organization targets the organization configured by a GitHubProviderConfig.
	// +optional
	Organization *GitHubOrganizationActionsTarget `json:"organization,omitempty"`
}

// Scope returns the configured target scope.
func (t GitHubActionsTarget) Scope() ActionsTargetScope {
	switch {
	case t.RepositoryRef != nil:
		return ActionsTargetScopeRepository
	case t.EnvironmentRef != nil:
		return ActionsTargetScopeEnvironment
	case t.Organization != nil:
		return ActionsTargetScopeOrganization
	default:
		return ""
	}
}
