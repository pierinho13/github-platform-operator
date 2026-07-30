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

import (
	"context"
	"errors"
)

// ErrNotFound indicates that a GitHub repository does not exist.
var ErrNotFound = errors.New("github repository not found")

// Repository contains the GitHub fields required by the controller.
type Repository struct {
	ID             int64
	HTMLURL        string
	Visibility     string
	Description    string
	Homepage       string
	Topics         []string
	HasIssues      bool
	HasProjects    bool
	HasWiki        bool
	HasDiscussions bool
}

// RepositoryUpdate contains optional repository fields to update.
// Nil fields are omitted from the GitHub API request.
type RepositoryUpdate struct {
	Visibility     *string `json:"visibility,omitempty"`
	Description    *string `json:"description,omitempty"`
	Homepage       *string `json:"homepage,omitempty"`
	HasIssues      *bool   `json:"has_issues,omitempty"`
	HasProjects    *bool   `json:"has_projects,omitempty"`
	HasWiki        *bool   `json:"has_wiki,omitempty"`
	HasDiscussions *bool   `json:"has_discussions,omitempty"`
}

// Empty reports whether the update contains no managed changes.
func (u RepositoryUpdate) Empty() bool {
	return u.Visibility == nil &&
		u.Description == nil &&
		u.Homepage == nil &&
		u.HasIssues == nil &&
		u.HasProjects == nil &&
		u.HasWiki == nil &&
		u.HasDiscussions == nil
}

// RepositoryClientFactory creates clients from provider credentials.
type RepositoryClientFactory interface {
	NewRepositoryClient(token, baseURL string) (RepositoryClient, error)
}

// RESTClientFactory creates REST-backed GitHub repository clients.
type RESTClientFactory struct{}

// NewRepositoryClient creates a REST-backed repository client.
func (RESTClientFactory) NewRepositoryClient(token, baseURL string) (RepositoryClient, error) {
	return NewRESTClient(token, baseURL)
}

// RepositoryClient defines the GitHub repository operations used by the controller.
type RepositoryClient interface {
	GetRepository(ctx context.Context, organization, name string) (*Repository, error)
	CreateRepository(ctx context.Context, organization, name string, private bool) (*Repository, error)
	UpdateRepository(
		ctx context.Context,
		organization string,
		name string,
		update RepositoryUpdate,
	) (*Repository, error)
	ReplaceRepositoryTopics(
		ctx context.Context,
		organization string,
		name string,
		topics []string,
	) ([]string, error)
	DeleteRepository(ctx context.Context, organization, name string) error
}
