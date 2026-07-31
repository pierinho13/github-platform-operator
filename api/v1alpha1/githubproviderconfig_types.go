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

const DefaultGitHubAPIURL = "https://api.github.com"

// NamespacedSecretKeyReference selects a key from a Secret in a specific namespace.
type NamespacedSecretKeyReference struct {
	// Namespace is the namespace containing the Secret.
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`

	// Name is the name of the Secret.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Key is the key containing the GitHub token or GitHub App private key.
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}

// GitHubAppCredentials configures authentication as a GitHub App installation.
type GitHubAppCredentials struct {
	// AppID is the GitHub App client ID or numeric app ID used as the JWT issuer.
	// GitHub recommends using the client ID.
	// +kubebuilder:validation:MinLength=1
	AppID string `json:"appID"`

	// InstallationID is the GitHub App installation ID.
	// +kubebuilder:validation:Minimum=1
	InstallationID int64 `json:"installationID"`

	// PrivateKeySecretRef references the PEM-encoded GitHub App private key.
	PrivateKeySecretRef NamespacedSecretKeyReference `json:"privateKeySecretRef"`
}

// GitHubProviderCredentials defines how the provider authenticates to GitHub.
// Exactly one of secretRef or githubApp must be configured.
// +kubebuilder:validation:XValidation:rule="has(self.secretRef) != has(self.githubApp)",message="exactly one of secretRef or githubApp must be configured"
type GitHubProviderCredentials struct {
	// SecretRef references a Secret containing a personal access token or
	// another already-issued GitHub token.
	// +optional
	SecretRef *NamespacedSecretKeyReference `json:"secretRef,omitempty"`

	// GitHubApp configures short-lived installation access tokens.
	// +optional
	GitHubApp *GitHubAppCredentials `json:"githubApp,omitempty"`
}

// GitHubProviderConfigSpec defines a reusable GitHub organization connection.
type GitHubProviderConfigSpec struct {
	// Organization is the GitHub organization managed through this provider.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="organization is immutable"
	Organization string `json:"organization"`

	// APIURL is the GitHub API base URL.
	// +kubebuilder:default="https://api.github.com"
	// +kubebuilder:validation:Pattern=`^https?://.+`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="apiURL is immutable"
	APIURL string `json:"apiURL,omitempty"`

	// Credentials defines how the provider authenticates to GitHub.
	Credentials GitHubProviderCredentials `json:"credentials"`

	// Suspended stops all remote reconciliation through this provider.
	// Kubernetes resources and finalizers are retained, and no GitHub API
	// requests are made until the provider is resumed.
	// +kubebuilder:default=false
	// +optional
	Suspended bool `json:"suspended,omitempty"`
}

// GitHubProviderConfigStatus defines the observed provider configuration state.
type GitHubProviderConfigStatus struct {
	// ObservedGeneration is the generation most recently validated.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions describe whether the referenced credentials are available.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=ghprovider
// +kubebuilder:printcolumn:name="Organization",type=string,JSONPath=`.spec.organization`
// +kubebuilder:printcolumn:name="API URL",type=string,JSONPath=`.spec.apiURL`
// +kubebuilder:printcolumn:name="Suspended",type=boolean,JSONPath=`.spec.suspended`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// GitHubProviderConfig is the Schema for reusable GitHub connections.
type GitHubProviderConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitHubProviderConfigSpec   `json:"spec,omitempty"`
	Status GitHubProviderConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GitHubProviderConfigList contains a list of GitHubProviderConfig.
type GitHubProviderConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitHubProviderConfig `json:"items"`
}
