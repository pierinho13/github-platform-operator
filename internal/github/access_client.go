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

package github

import "context"

// CollaboratorAccess describes direct repository access for a GitHub user.
type CollaboratorAccess struct {
	Permission        string
	InvitationPending bool
	InvitationID      int64
}

// RepositoryAccessClientFactory creates clients used to manage teams and collaborators.
type RepositoryAccessClientFactory interface {
	NewRepositoryAccessClient(token, baseURL string) (RepositoryAccessClient, error)
}

// NewRepositoryAccessClient creates a REST-backed repository access client.
func (RESTClientFactory) NewRepositoryAccessClient(
	token string,
	baseURL string,
) (RepositoryAccessClient, error) {
	return NewRESTClient(token, baseURL)
}

// RepositoryAccessClient defines GitHub team and collaborator access operations.
type RepositoryAccessClient interface {
	GetRepository(ctx context.Context, organization, name string) (*Repository, error)

	GetTeamRepositoryPermission(
		ctx context.Context,
		organization string,
		teamSlug string,
		repositoryOwner string,
		repositoryName string,
	) (string, error)
	SetTeamRepositoryPermission(
		ctx context.Context,
		organization string,
		teamSlug string,
		repositoryOwner string,
		repositoryName string,
		permission string,
	) error
	RemoveTeamRepository(
		ctx context.Context,
		organization string,
		teamSlug string,
		repositoryOwner string,
		repositoryName string,
	) error

	GetCollaboratorAccess(
		ctx context.Context,
		repositoryOwner string,
		repositoryName string,
		username string,
	) (*CollaboratorAccess, error)
	SetCollaboratorPermission(
		ctx context.Context,
		repositoryOwner string,
		repositoryName string,
		username string,
		permission string,
	) (*CollaboratorAccess, error)
	UpdateRepositoryInvitation(
		ctx context.Context,
		repositoryOwner string,
		repositoryName string,
		invitationID int64,
		permission string,
	) (*CollaboratorAccess, error)
	RemoveCollaboratorAccess(
		ctx context.Context,
		repositoryOwner string,
		repositoryName string,
		username string,
		invitationID int64,
	) error
}
