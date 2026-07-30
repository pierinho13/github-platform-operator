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

// GitHubActionsSecretSpec defines a GitHub Actions secret sourced from Kubernetes.
type GitHubActionsSecretSpec struct {
	// Target selects a repository, environment, or organization.
	Target GitHubActionsTarget `json:"target"`

	// Name is the GitHub Actions secret name.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^[A-Za-z_][A-Za-z0-9_]*$`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// ValueFrom selects the Kubernetes Secret value. Plaintext values are not supported.
	ValueFrom ActionsValueSource `json:"valueFrom"`

	// DeletionPolicy controls whether deleting this resource revokes the GitHub secret.
	// +kubebuilder:default=Orphan
	DeletionPolicy ActionsResourceDeletionPolicy `json:"deletionPolicy,omitempty"`
}

// EffectiveDeletionPolicy returns the configured policy or the safe Orphan default.
func (s GitHubActionsSecretSpec) EffectiveDeletionPolicy() ActionsResourceDeletionPolicy {
	return EffectiveActionsResourceDeletionPolicy(s.DeletionPolicy)
}

// GitHubActionsSecretStatus defines the observed secret synchronization state.
type GitHubActionsSecretStatus struct {
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
	// SecretName is the synchronized GitHub secret name.
	// +optional
	SecretName string `json:"secretName,omitempty"`
	// SourceSecretUID identifies the Kubernetes Secret used during synchronization.
	// +optional
	SourceSecretUID string `json:"sourceSecretUid,omitempty"`
	// SourceSecretResourceVersion is the synchronized Kubernetes Secret resource version.
	// +optional
	SourceSecretResourceVersion string `json:"sourceSecretResourceVersion,omitempty"`
	// RemoteUpdatedAt is the GitHub metadata timestamp observed after synchronization.
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
// +kubebuilder:resource:shortName=ghsecret
// +kubebuilder:printcolumn:name="Scope",type=string,JSONPath=`.status.targetScope`
// +kubebuilder:printcolumn:name="Repository",type=string,JSONPath=`.status.repository`
// +kubebuilder:printcolumn:name="Environment",type=string,JSONPath=`.status.environment`
// +kubebuilder:printcolumn:name="Secret",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="Deletion Policy",type=string,JSONPath=`.spec.deletionPolicy`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// GitHubActionsSecret synchronizes a Kubernetes Secret value into GitHub Actions.
type GitHubActionsSecret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitHubActionsSecretSpec   `json:"spec,omitempty"`
	Status GitHubActionsSecretStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GitHubActionsSecretList contains a list of GitHubActionsSecret.
type GitHubActionsSecretList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitHubActionsSecret `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GitHubActionsSecret{}, &GitHubActionsSecretList{})
}
