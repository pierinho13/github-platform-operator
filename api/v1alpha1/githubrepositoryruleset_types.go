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
	"k8s.io/apimachinery/pkg/runtime"
)

// GitHubRulesetTarget identifies the object family controlled by a ruleset.
// +kubebuilder:validation:Enum=branch;tag;push
type GitHubRulesetTarget string

const (
	GitHubRulesetTargetBranch GitHubRulesetTarget = "branch"
	GitHubRulesetTargetTag    GitHubRulesetTarget = "tag"
	GitHubRulesetTargetPush   GitHubRulesetTarget = "push"
)

// GitHubRulesetEnforcement controls whether a ruleset is active.
// +kubebuilder:validation:Enum=disabled;active;evaluate
type GitHubRulesetEnforcement string

const (
	GitHubRulesetEnforcementDisabled GitHubRulesetEnforcement = "disabled"
	GitHubRulesetEnforcementActive   GitHubRulesetEnforcement = "active"
	GitHubRulesetEnforcementEvaluate GitHubRulesetEnforcement = "evaluate"
)

// GitHubRulesetBypassActorType identifies an actor allowed to bypass a ruleset.
// +kubebuilder:validation:Enum=Integration;OrganizationAdmin;RepositoryRole;Team;DeployKey;User
type GitHubRulesetBypassActorType string

const (
	GitHubRulesetBypassActorIntegration       GitHubRulesetBypassActorType = "Integration"
	GitHubRulesetBypassActorOrganizationAdmin GitHubRulesetBypassActorType = "OrganizationAdmin"
	GitHubRulesetBypassActorRepositoryRole    GitHubRulesetBypassActorType = "RepositoryRole"
	GitHubRulesetBypassActorTeam              GitHubRulesetBypassActorType = "Team"
	GitHubRulesetBypassActorDeployKey         GitHubRulesetBypassActorType = "DeployKey"
	GitHubRulesetBypassActorUser              GitHubRulesetBypassActorType = "User"
)

// GitHubRulesetBypassMode controls when an actor may bypass a ruleset.
// +kubebuilder:validation:Enum=always;pull_request;exempt
type GitHubRulesetBypassMode string

const (
	GitHubRulesetBypassModeAlways      GitHubRulesetBypassMode = "always"
	GitHubRulesetBypassModePullRequest GitHubRulesetBypassMode = "pull_request"
	GitHubRulesetBypassModeExempt      GitHubRulesetBypassMode = "exempt"
)

// GitHubRulesetRuleType is a GitHub repository ruleset rule type.
//
// Parameters remain raw JSON because GitHub evolves individual rule schemas
// independently. This keeps every current rule option available without
// forcing CRD upgrades whenever GitHub adds a parameter.
// +kubebuilder:validation:Enum=creation;update;deletion;required_linear_history;merge_queue;required_deployments;required_signatures;pull_request;required_status_checks;non_fast_forward;commit_message_pattern;commit_author_email_pattern;committer_email_pattern;branch_name_pattern;tag_name_pattern;workflows;code_scanning;copilot_code_review;license_compliance_scanning;file_path_restriction;max_file_path_length;file_extension_restriction;max_file_size
type GitHubRulesetRuleType string

const (
	GitHubRulesetRuleCreation                  GitHubRulesetRuleType = "creation"
	GitHubRulesetRuleUpdate                    GitHubRulesetRuleType = "update"
	GitHubRulesetRuleDeletion                  GitHubRulesetRuleType = "deletion"
	GitHubRulesetRuleRequiredLinearHistory     GitHubRulesetRuleType = "required_linear_history"
	GitHubRulesetRuleMergeQueue                GitHubRulesetRuleType = "merge_queue"
	GitHubRulesetRuleRequiredDeployments       GitHubRulesetRuleType = "required_deployments"
	GitHubRulesetRuleRequiredSignatures        GitHubRulesetRuleType = "required_signatures"
	GitHubRulesetRulePullRequest               GitHubRulesetRuleType = "pull_request"
	GitHubRulesetRuleRequiredStatusChecks      GitHubRulesetRuleType = "required_status_checks"
	GitHubRulesetRuleNonFastForward            GitHubRulesetRuleType = "non_fast_forward"
	GitHubRulesetRuleCommitMessagePattern      GitHubRulesetRuleType = "commit_message_pattern"
	GitHubRulesetRuleCommitAuthorEmailPattern  GitHubRulesetRuleType = "commit_author_email_pattern"
	GitHubRulesetRuleCommitterEmailPattern     GitHubRulesetRuleType = "committer_email_pattern"
	GitHubRulesetRuleBranchNamePattern         GitHubRulesetRuleType = "branch_name_pattern"
	GitHubRulesetRuleTagNamePattern            GitHubRulesetRuleType = "tag_name_pattern"
	GitHubRulesetRuleWorkflows                 GitHubRulesetRuleType = "workflows"
	GitHubRulesetRuleCodeScanning              GitHubRulesetRuleType = "code_scanning"
	GitHubRulesetRuleCopilotCodeReview         GitHubRulesetRuleType = "copilot_code_review"
	GitHubRulesetRuleLicenseComplianceScanning GitHubRulesetRuleType = "license_compliance_scanning"
	GitHubRulesetRuleFilePathRestriction       GitHubRulesetRuleType = "file_path_restriction"
	GitHubRulesetRuleMaxFilePathLength         GitHubRulesetRuleType = "max_file_path_length"
	GitHubRulesetRuleFileExtensionRestriction  GitHubRulesetRuleType = "file_extension_restriction"
	GitHubRulesetRuleMaxFileSize               GitHubRulesetRuleType = "max_file_size"
)

// GitHubRulesetBypassActor defines an actor that can bypass a ruleset.
// +kubebuilder:validation:XValidation:rule="(self.actorType in ['Integration', 'RepositoryRole', 'Team', 'User']) ? has(self.actorID) : !has(self.actorID)",message="actorID is required for Integration, RepositoryRole, Team and User, and must be omitted for OrganizationAdmin and DeployKey"
type GitHubRulesetBypassActor struct {
	// ActorID is required for Integration, RepositoryRole, Team, and User.
	// It is ignored for OrganizationAdmin and must be null for DeployKey.
	// +kubebuilder:validation:Minimum=1
	// +optional
	ActorID *int64 `json:"actorID,omitempty"`

	// ActorType is the GitHub actor type.
	ActorType GitHubRulesetBypassActorType `json:"actorType"`

	// BypassMode controls when bypass is permitted.
	// +kubebuilder:default=always
	// +optional
	BypassMode GitHubRulesetBypassMode `json:"bypassMode,omitempty"`
}

// GitHubRulesetRefNameCondition selects refs by include and exclude patterns.
type GitHubRulesetRefNameCondition struct {
	// Include contains ref names or patterns. ~DEFAULT_BRANCH and ~ALL are supported.
	// +kubebuilder:validation:MinItems=1
	Include []string `json:"include"`

	// Exclude contains ref names or patterns that must not match.
	// +optional
	Exclude []string `json:"exclude,omitempty"`
}

// GitHubRulesetConditions defines the conditions applied to a repository ruleset.
type GitHubRulesetConditions struct {
	// RefName selects branch or tag refs.
	// +optional
	RefName *GitHubRulesetRefNameCondition `json:"refName,omitempty"`
}

// GitHubRulesetRule defines one rule. Parameters are passed directly to the
// GitHub API and therefore support every current parameter shape.
// Examples include pull_request, merge_queue, required_status_checks,
// workflows, code_scanning, pattern rules, and push restriction rules.
type GitHubRulesetRule struct {
	// Type is the GitHub rule type.
	Type GitHubRulesetRuleType `json:"type"`

	// Parameters contains the rule-specific GitHub API object.
	// Omit it for parameterless rules.
	// +optional
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`
}

// GitHubRulesetDeletionPolicy controls deletion behavior.
// +kubebuilder:validation:Enum=Orphan;Delete
type GitHubRulesetDeletionPolicy string

const (
	GitHubRulesetDeletionPolicyOrphan GitHubRulesetDeletionPolicy = "Orphan"
	GitHubRulesetDeletionPolicyDelete GitHubRulesetDeletionPolicy = "Delete"
)

// GitHubRepositoryRulesetSpec defines a repository ruleset.
type GitHubRepositoryRulesetSpec struct {
	// RepositoryRef references the repository custom resource.
	RepositoryRef GitHubRepositoryReference `json:"repositoryRef"`

	// Name is the GitHub ruleset name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Target is branch, tag, or push.
	// +kubebuilder:default=branch
	Target GitHubRulesetTarget `json:"target,omitempty"`

	// Enforcement is disabled, active, or evaluate.
	Enforcement GitHubRulesetEnforcement `json:"enforcement"`

	// BypassActors lists actors that may bypass the ruleset. Omit the field to leave
	// bypass actors unmanaged, or set an empty list to remove all bypass actors.
	// +optional
	BypassActors []GitHubRulesetBypassActor `json:"bypassActors,omitempty"`

	// Conditions selects refs affected by the ruleset. When omitted, existing
	// conditions are not compared or changed.
	// +optional
	Conditions *GitHubRulesetConditions `json:"conditions,omitempty"`

	// Rules contains the complete desired ruleset rule list.
	// +kubebuilder:validation:MinItems=1
	Rules []GitHubRulesetRule `json:"rules"`

	// DeletionPolicy controls whether deleting this custom resource deletes
	// the GitHub ruleset.
	// +kubebuilder:default=Orphan
	// +optional
	DeletionPolicy GitHubRulesetDeletionPolicy `json:"deletionPolicy,omitempty"`
}

// EffectiveTarget returns the configured target or branch by default.
func (s GitHubRepositoryRulesetSpec) EffectiveTarget() GitHubRulesetTarget {
	if s.Target == "" {
		return GitHubRulesetTargetBranch
	}
	return s.Target
}

// EffectiveDeletionPolicy returns Orphan when no policy is configured.
func (s GitHubRepositoryRulesetSpec) EffectiveDeletionPolicy() GitHubRulesetDeletionPolicy {
	if s.DeletionPolicy == "" {
		return GitHubRulesetDeletionPolicyOrphan
	}
	return s.DeletionPolicy
}

// GitHubRepositoryRulesetStatus defines the observed ruleset state.
type GitHubRepositoryRulesetStatus struct {
	// ObservedGeneration is the generation most recently reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ProviderConfigRef is the resolved provider.
	// +optional
	ProviderConfigRef string `json:"providerConfigRef,omitempty"`

	// Organization is the resolved GitHub organization.
	// +optional
	Organization string `json:"organization,omitempty"`

	// Repository is the resolved GitHub repository.
	// +optional
	Repository string `json:"repository,omitempty"`

	// RulesetID is the GitHub ruleset ID.
	// +optional
	RulesetID int64 `json:"rulesetID,omitempty"`

	// RulesetName is the observed GitHub ruleset name.
	// +optional
	RulesetName string `json:"rulesetName,omitempty"`

	// Target is the observed target.
	// +optional
	Target GitHubRulesetTarget `json:"target,omitempty"`

	// Enforcement is the observed enforcement state.
	// +optional
	Enforcement GitHubRulesetEnforcement `json:"enforcement,omitempty"`

	// URL is the GitHub web URL for the ruleset when GitHub returns it.
	// +optional
	URL string `json:"url,omitempty"`

	// Conditions describe whether the ruleset is synchronized.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ghruleset
// +kubebuilder:printcolumn:name="Repository",type=string,JSONPath=`.status.repository`
// +kubebuilder:printcolumn:name="Ruleset",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.spec.target`
// +kubebuilder:printcolumn:name="Enforcement",type=string,JSONPath=`.spec.enforcement`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// GitHubRepositoryRuleset is the Schema for GitHub repository rulesets.
type GitHubRepositoryRuleset struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitHubRepositoryRulesetSpec   `json:"spec,omitempty"`
	Status GitHubRepositoryRulesetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GitHubRepositoryRulesetList contains a list of GitHubRepositoryRuleset.
type GitHubRepositoryRulesetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitHubRepositoryRuleset `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GitHubRepositoryRuleset{}, &GitHubRepositoryRulesetList{})
}
