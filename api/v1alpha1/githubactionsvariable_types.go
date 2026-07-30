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

// GitHubActionsVariableSpec defines a GitHub Actions variable sourced from Kubernetes.
type GitHubActionsVariableSpec struct {
	// Target selects a repository, environment, or organization.
	Target GitHubActionsTarget `json:"target"`

	// Name is the GitHub Actions variable name.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^[A-Za-z_][A-Za-z0-9_]*$`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// ValueFrom selects the Kubernetes Secret value. Variables must contain only
	// non-sensitive values because GitHub returns variable values through its API.
	ValueFrom ActionsValueSource `json:"valueFrom"`

	// DeletionPolicy controls whether deleting this resource revokes the GitHub variable.
	// +kubebuilder:default=Orphan
	DeletionPolicy ActionsResourceDeletionPolicy `json:"deletionPolicy,omitempty"`
}

// EffectiveDeletionPolicy returns the configured policy or the safe Orphan default.
func (s GitHubActionsVariableSpec) EffectiveDeletionPolicy() ActionsResourceDeletionPolicy {
	return EffectiveActionsResourceDeletionPolicy(s.DeletionPolicy)
}

// GitHubActionsVariableStatus defines the observed variable synchronization state.
type GitHubActionsVariableStatus struct {
	// TargetScope is repository, environment, or organization.
	// +optional
	TargetScope ActionsTargetScope `json:"targetScope,omitempty"`
	// ProviderConfigRef is the resolved provider.
	// +optional
	ProviderConfigRef string `json:"providerConfigRef,omitempty"`
	// Organization is the resolved organization.
	// +optional
	Organization string `json:"organization,omitempty"`
	// Repository is the resolved repository name when applicable.
	// +optional
	Repository string `json:"repository,omitempty"`
	// Environment is the resolved environment name when applicable.
	// +optional
	Environment string `json:"environment,omitempty"`
	// VariableName is the synchronized GitHub variable name.
	// +optional
	VariableName string `json:"variableName,omitempty"`
	// SourceSecretUID identifies the Kubernetes Secret used during synchronization.
	// +optional
	SourceSecretUID string `json:"sourceSecretUid,omitempty"`
	// SourceSecretResourceVersion is the synchronized Kubernetes Secret resource version.
	// +optional
	SourceSecretResourceVersion string `json:"sourceSecretResourceVersion,omitempty"`
	// RemoteUpdatedAt is the GitHub timestamp observed after synchronization.
	// +optional
	RemoteUpdatedAt string `json:"remoteUpdatedAt,omitempty"`
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
// +kubebuilder:resource:shortName=ghvar
// +kubebuilder:printcolumn:name="Scope",type=string,JSONPath=`.status.targetScope`
// +kubebuilder:printcolumn:name="Repository",type=string,JSONPath=`.status.repository`
// +kubebuilder:printcolumn:name="Environment",type=string,JSONPath=`.status.environment`
// +kubebuilder:printcolumn:name="Variable",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="Deletion Policy",type=string,JSONPath=`.spec.deletionPolicy`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// GitHubActionsVariable synchronizes a Kubernetes Secret value into a GitHub Actions variable.
type GitHubActionsVariable struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitHubActionsVariableSpec   `json:"spec,omitempty"`
	Status GitHubActionsVariableStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GitHubActionsVariableList contains a list of GitHubActionsVariable.
type GitHubActionsVariableList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitHubActionsVariable `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GitHubActionsVariable{}, &GitHubActionsVariableList{})
}
