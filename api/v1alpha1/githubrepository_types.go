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

const DefaultGitHubProviderConfigName = "default"

// RepositoryVisibility defines the visibility supported by GitHub repositories.
// Internal visibility requires GitHub Enterprise.
// +kubebuilder:validation:Enum=public;private;internal
type RepositoryVisibility string

const (
	// RepositoryVisibilityPublic makes the repository publicly accessible.
	RepositoryVisibilityPublic RepositoryVisibility = "public"

	// RepositoryVisibilityPrivate restricts access to authorized users.
	RepositoryVisibilityPrivate RepositoryVisibility = "private"

	// RepositoryVisibilityInternal makes the repository visible to enterprise members.
	RepositoryVisibilityInternal RepositoryVisibility = "internal"
)

// RepositoryFeatures defines optional GitHub repository features managed by the operator.
// A nil field is observed but not reconciled.
type RepositoryFeatures struct {
	// Issues controls whether GitHub Issues are enabled.
	// +optional
	Issues *bool `json:"issues,omitempty"`

	// Projects controls whether GitHub Projects are enabled.
	// +optional
	Projects *bool `json:"projects,omitempty"`

	// Wiki controls whether the repository wiki is enabled.
	// +optional
	Wiki *bool `json:"wiki,omitempty"`

	// Discussions controls whether GitHub Discussions are enabled.
	// +optional
	Discussions *bool `json:"discussions,omitempty"`
}

// RepositoryFeaturesStatus contains the repository feature values observed in GitHub.
type RepositoryFeaturesStatus struct {
	Issues      bool `json:"issues"`
	Projects    bool `json:"projects"`
	Wiki        bool `json:"wiki"`
	Discussions bool `json:"discussions"`
}

// RepositoryTemplate configures creation from an existing GitHub template repository.
// It is only used when the remote repository does not already exist.
type RepositoryTemplate struct {
	// Owner is the user or organization that owns the template repository.
	// +kubebuilder:validation:MinLength=1
	Owner string `json:"owner"`

	// Repository is the template repository name.
	// +kubebuilder:validation:MinLength=1
	Repository string `json:"repository"`

	// IncludeAllBranches copies every branch instead of only the default branch.
	// +optional
	IncludeAllBranches bool `json:"includeAllBranches,omitempty"`
}

// RepositoryMergeCommitTitle defines the default merge-commit title format.
// +kubebuilder:validation:Enum=PR_TITLE;MERGE_MESSAGE
type RepositoryMergeCommitTitle string

const (
	RepositoryMergeCommitTitlePRTitle      RepositoryMergeCommitTitle = "PR_TITLE"
	RepositoryMergeCommitTitleMergeMessage RepositoryMergeCommitTitle = "MERGE_MESSAGE"
)

// RepositoryMergeCommitMessage defines the default merge-commit message format.
// +kubebuilder:validation:Enum=PR_BODY;PR_TITLE;BLANK
type RepositoryMergeCommitMessage string

const (
	RepositoryMergeCommitMessagePRBody  RepositoryMergeCommitMessage = "PR_BODY"
	RepositoryMergeCommitMessagePRTitle RepositoryMergeCommitMessage = "PR_TITLE"
	RepositoryMergeCommitMessageBlank   RepositoryMergeCommitMessage = "BLANK"
)

// RepositorySquashMergeCommitTitle defines the default squash-merge title format.
// +kubebuilder:validation:Enum=PR_TITLE;COMMIT_OR_PR_TITLE
type RepositorySquashMergeCommitTitle string

const (
	RepositorySquashMergeCommitTitlePRTitle         RepositorySquashMergeCommitTitle = "PR_TITLE"
	RepositorySquashMergeCommitTitleCommitOrPRTitle RepositorySquashMergeCommitTitle = "COMMIT_OR_PR_TITLE"
)

// RepositorySquashMergeCommitMessage defines the default squash-merge message format.
// +kubebuilder:validation:Enum=PR_BODY;COMMIT_MESSAGES;BLANK
type RepositorySquashMergeCommitMessage string

const (
	RepositorySquashMergeCommitMessagePRBody         RepositorySquashMergeCommitMessage = "PR_BODY"
	RepositorySquashMergeCommitMessageCommitMessages RepositorySquashMergeCommitMessage = "COMMIT_MESSAGES"
	RepositorySquashMergeCommitMessageBlank          RepositorySquashMergeCommitMessage = "BLANK"
)

// RepositoryMergeOptions contains optional pull-request merge settings.
// Only fields explicitly set inside this object are reconciled.
type RepositoryMergeOptions struct {
	// AllowAutoMerge controls whether pull requests can use auto-merge.
	// +optional
	AllowAutoMerge *bool `json:"allowAutoMerge,omitempty"`

	// AllowMergeCommit controls whether merge commits are allowed.
	// +optional
	AllowMergeCommit *bool `json:"allowMergeCommit,omitempty"`

	// AllowRebaseMerge controls whether rebase merges are allowed.
	// +optional
	AllowRebaseMerge *bool `json:"allowRebaseMerge,omitempty"`

	// AllowSquashMerge controls whether squash merges are allowed.
	// +optional
	AllowSquashMerge *bool `json:"allowSquashMerge,omitempty"`

	// MergeCommitTitle controls the default title for merge commits.
	// +optional
	MergeCommitTitle *RepositoryMergeCommitTitle `json:"mergeCommitTitle,omitempty"`

	// MergeCommitMessage controls the default body for merge commits.
	// +optional
	MergeCommitMessage *RepositoryMergeCommitMessage `json:"mergeCommitMessage,omitempty"`

	// SquashMergeCommitTitle controls the default title for squash merges.
	// +optional
	SquashMergeCommitTitle *RepositorySquashMergeCommitTitle `json:"squashMergeCommitTitle,omitempty"`

	// SquashMergeCommitMessage controls the default body for squash merges.
	// +optional
	SquashMergeCommitMessage *RepositorySquashMergeCommitMessage `json:"squashMergeCommitMessage,omitempty"`
}

// RepositoryMergeOptionsStatus contains merge settings observed in GitHub.
type RepositoryMergeOptionsStatus struct {
	AllowAutoMerge           bool                               `json:"allowAutoMerge"`
	AllowMergeCommit         bool                               `json:"allowMergeCommit"`
	AllowRebaseMerge         bool                               `json:"allowRebaseMerge"`
	AllowSquashMerge         bool                               `json:"allowSquashMerge"`
	MergeCommitTitle         RepositoryMergeCommitTitle         `json:"mergeCommitTitle,omitempty"`
	MergeCommitMessage       RepositoryMergeCommitMessage       `json:"mergeCommitMessage,omitempty"`
	SquashMergeCommitTitle   RepositorySquashMergeCommitTitle   `json:"squashMergeCommitTitle,omitempty"`
	SquashMergeCommitMessage RepositorySquashMergeCommitMessage `json:"squashMergeCommitMessage,omitempty"`
}

// RepositoryDeletionPolicy defines what happens to the remote repository when
// the GitHubRepository custom resource is deleted.
// +kubebuilder:validation:Enum=Orphan;Archive;Delete
type RepositoryDeletionPolicy string

const (
	// RepositoryDeletionPolicyOrphan keeps the GitHub repository when the custom resource is deleted.
	RepositoryDeletionPolicyOrphan RepositoryDeletionPolicy = "Orphan"

	// RepositoryDeletionPolicyArchive archives the GitHub repository when the custom resource is deleted.
	RepositoryDeletionPolicyArchive RepositoryDeletionPolicy = "Archive"

	// RepositoryDeletionPolicyDelete permanently deletes the GitHub repository with the custom resource.
	RepositoryDeletionPolicyDelete RepositoryDeletionPolicy = "Delete"
)

// GitHubRepositorySpec defines the desired state of GitHubRepository.
// +kubebuilder:validation:XValidation:rule="!(has(self.template) && has(self.autoInit))",message="template and autoInit are mutually exclusive"
type GitHubRepositorySpec struct {
	// ProviderConfigRef references the cluster-scoped GitHubProviderConfig.
	// +kubebuilder:default=default
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="providerConfigRef is immutable"
	ProviderConfigRef string `json:"providerConfigRef,omitempty"`

	// Organization is deprecated and ignored. Configure the organization through
	// GitHubProviderConfig instead. It is temporarily retained for v1alpha1 migration.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="organization is immutable"
	Organization string `json:"organization,omitempty"`

	// Name is the name of the GitHub repository.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Visibility determines whether the operator manages the repository visibility.
	// When omitted, new repositories are created as private, while existing repositories
	// keep their current visibility. Internal visibility requires GitHub Enterprise.
	// +optional
	Visibility *RepositoryVisibility `json:"visibility,omitempty"`

	// Description is the short repository description. When omitted, the operator
	// preserves the current value. Set it to an empty string to clear it.
	// +optional
	Description *string `json:"description,omitempty"`

	// Homepage is the repository website URL. When omitted, the operator preserves
	// the current value. Set it to an empty string to clear it.
	// +optional
	Homepage *string `json:"homepage,omitempty"`

	// Topics is the complete set of repository topics managed by the operator.
	// When omitted, existing topics are preserved. Set an empty list to clear all topics.
	// GitHub stores topic names in lowercase.
	// +optional
	// +kubebuilder:validation:MaxItems=20
	// +listType=set
	Topics *[]string `json:"topics,omitempty"`

	// Features contains optional repository features. Only fields explicitly set
	// inside this object are reconciled.
	// +optional
	Features *RepositoryFeatures `json:"features,omitempty"`

	// AutoInit creates an initial commit with an empty README. It is only used when
	// creating a repository and cannot be changed afterwards.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="autoInit is immutable"
	AutoInit *bool `json:"autoInit,omitempty"`

	// Template creates the repository from an existing template. It is only used
	// during creation and cannot be changed afterwards.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="template is immutable"
	Template *RepositoryTemplate `json:"template,omitempty"`

	// DeleteBranchOnMerge controls automatic deletion of merged head branches.
	// +optional
	DeleteBranchOnMerge *bool `json:"deleteBranchOnMerge,omitempty"`

	// VulnerabilityAlerts controls dependency graph vulnerability alerts. This
	// setting requires repository Administration permission.
	// +optional
	VulnerabilityAlerts *bool `json:"vulnerabilityAlerts,omitempty"`

	// IsTemplate controls whether this repository can be used as a template.
	// +optional
	IsTemplate *bool `json:"isTemplate,omitempty"`

	// MergeOptions contains optional pull-request merge settings.
	// +optional
	MergeOptions *RepositoryMergeOptions `json:"mergeOptions,omitempty"`

	// DeletionPolicy determines whether deleting the custom resource orphans,
	// archives or permanently deletes the remote GitHub repository.
	// +kubebuilder:default=Orphan
	DeletionPolicy RepositoryDeletionPolicy `json:"deletionPolicy,omitempty"`
}

// EffectiveProviderConfigRef returns the configured provider or the default provider name.
func (s GitHubRepositorySpec) EffectiveProviderConfigRef() string {
	if s.ProviderConfigRef == "" {
		return DefaultGitHubProviderConfigName
	}

	return s.ProviderConfigRef
}

// EffectiveDeletionPolicy returns the configured policy or the safe Orphan default.
func (s GitHubRepositorySpec) EffectiveDeletionPolicy() RepositoryDeletionPolicy {
	if s.DeletionPolicy == "" {
		return RepositoryDeletionPolicyOrphan
	}

	return s.DeletionPolicy
}

// EffectiveVisibilityForCreation returns the requested visibility for a new
// repository. Repositories are created private when visibility is omitted.
func (s GitHubRepositorySpec) EffectiveVisibilityForCreation() RepositoryVisibility {
	if s.Visibility == nil {
		return RepositoryVisibilityPrivate
	}

	return *s.Visibility
}

// EffectiveAutoInit returns whether a new repository should contain an initial commit.
func (s GitHubRepositorySpec) EffectiveAutoInit() bool {
	return s.AutoInit != nil && *s.AutoInit
}

// GitHubRepositoryStatus defines the observed state of GitHubRepository.
type GitHubRepositoryStatus struct {
	// ProviderConfigRef is the provider configuration used during reconciliation.
	// +optional
	ProviderConfigRef string `json:"providerConfigRef,omitempty"`

	// Organization is the GitHub organization last observed from the provider configuration.
	// +optional
	Organization string `json:"organization,omitempty"`

	// RepositoryID is the numeric identifier assigned by GitHub.
	// +optional
	RepositoryID int64 `json:"repositoryId,omitempty"`

	// URL is the HTML URL of the GitHub repository.
	// +optional
	URL string `json:"url,omitempty"`

	// Visibility is the repository visibility last observed in GitHub.
	// +optional
	Visibility RepositoryVisibility `json:"visibility,omitempty"`

	// Description is the repository description last observed in GitHub.
	// +optional
	Description string `json:"description,omitempty"`

	// Homepage is the repository homepage last observed in GitHub.
	// +optional
	Homepage string `json:"homepage,omitempty"`

	// Topics is the repository topic set last observed in GitHub.
	// +optional
	// +listType=set
	Topics []string `json:"topics,omitempty"`

	// Features contains repository feature values last observed in GitHub.
	// +optional
	Features *RepositoryFeaturesStatus `json:"features,omitempty"`

	// DeleteBranchOnMerge is the observed merged-branch cleanup setting.
	// +optional
	DeleteBranchOnMerge bool `json:"deleteBranchOnMerge,omitempty"`

	// VulnerabilityAlerts is populated when spec.vulnerabilityAlerts is managed.
	// +optional
	VulnerabilityAlerts *bool `json:"vulnerabilityAlerts,omitempty"`

	// IsTemplate is the observed template-repository setting.
	// +optional
	IsTemplate bool `json:"isTemplate,omitempty"`

	// Archived indicates whether GitHub reports the repository as archived.
	// +optional
	Archived bool `json:"archived,omitempty"`

	// MergeOptions contains merge settings observed in GitHub.
	// +optional
	MergeOptions *RepositoryMergeOptionsStatus `json:"mergeOptions,omitempty"`

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
// +kubebuilder:printcolumn:name="Provider",type=string,JSONPath=`.spec.providerConfigRef`
// +kubebuilder:printcolumn:name="Organization",type=string,JSONPath=`.status.organization`
// +kubebuilder:printcolumn:name="Repository",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="Visibility",type=string,JSONPath=`.status.visibility`
// +kubebuilder:printcolumn:name="Deletion Policy",type=string,JSONPath=`.spec.deletionPolicy`
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
