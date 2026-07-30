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

// RepositoryPermission defines the standard GitHub repository roles supported
// by repository access resources.
// +kubebuilder:validation:Enum=pull;triage;push;maintain;admin
type RepositoryPermission string

const (
	RepositoryPermissionPull     RepositoryPermission = "pull"
	RepositoryPermissionTriage   RepositoryPermission = "triage"
	RepositoryPermissionPush     RepositoryPermission = "push"
	RepositoryPermissionMaintain RepositoryPermission = "maintain"
	RepositoryPermissionAdmin    RepositoryPermission = "admin"
)

// RepositoryAccessDeletionPolicy defines what happens to the GitHub access
// assignment when its Kubernetes custom resource is deleted.
// +kubebuilder:validation:Enum=Orphan;Revoke
type RepositoryAccessDeletionPolicy string

const (
	// RepositoryAccessDeletionPolicyOrphan keeps the GitHub access assignment.
	RepositoryAccessDeletionPolicyOrphan RepositoryAccessDeletionPolicy = "Orphan"

	// RepositoryAccessDeletionPolicyRevoke removes the GitHub access assignment.
	RepositoryAccessDeletionPolicyRevoke RepositoryAccessDeletionPolicy = "Revoke"
)

// GitHubRepositoryReference references a GitHubRepository in the same namespace.
type GitHubRepositoryReference struct {
	// Name is the Kubernetes name of the GitHubRepository.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="repositoryRef.name is immutable"
	Name string `json:"name"`
}

// EffectiveRepositoryAccessDeletionPolicy returns the configured policy or the
// safe Orphan default.
func EffectiveRepositoryAccessDeletionPolicy(
	policy RepositoryAccessDeletionPolicy,
) RepositoryAccessDeletionPolicy {
	if policy == "" {
		return RepositoryAccessDeletionPolicyOrphan
	}

	return policy
}
