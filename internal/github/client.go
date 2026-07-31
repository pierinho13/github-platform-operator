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
	"net/http"
)

// ErrNotFound indicates that a GitHub resource does not exist.
var ErrNotFound = errors.New("github resource not found")

// Repository contains the GitHub fields required by the controller.
type Repository struct {
	ID                       int64
	HTMLURL                  string
	Visibility               string
	Description              string
	Homepage                 string
	Topics                   []string
	HasIssues                bool
	HasProjects              bool
	HasWiki                  bool
	HasDiscussions           bool
	DeleteBranchOnMerge      bool
	IsTemplate               bool
	Archived                 bool
	AllowAutoMerge           bool
	AllowMergeCommit         bool
	AllowRebaseMerge         bool
	AllowSquashMerge         bool
	MergeCommitTitle         string
	MergeCommitMessage       string
	SquashMergeCommitTitle   string
	SquashMergeCommitMessage string
	VulnerabilityAlerts      *bool
}

// RepositoryCreate contains settings used only while creating a repository.
type RepositoryCreate struct {
	Name       string
	Visibility string
	AutoInit   bool
	Template   *RepositoryTemplateCreate
}

// RepositoryTemplateCreate identifies a template used to generate a repository.
type RepositoryTemplateCreate struct {
	Owner              string
	Repository         string
	IncludeAllBranches bool
}

// RepositoryUpdate contains optional repository fields to update.
// Nil fields are omitted from the GitHub API request.
type RepositoryUpdate struct {
	Visibility               *string `json:"visibility,omitempty"`
	Description              *string `json:"description,omitempty"`
	Homepage                 *string `json:"homepage,omitempty"`
	HasIssues                *bool   `json:"has_issues,omitempty"`
	HasProjects              *bool   `json:"has_projects,omitempty"`
	HasWiki                  *bool   `json:"has_wiki,omitempty"`
	HasDiscussions           *bool   `json:"has_discussions,omitempty"`
	DeleteBranchOnMerge      *bool   `json:"delete_branch_on_merge,omitempty"`
	IsTemplate               *bool   `json:"is_template,omitempty"`
	AllowAutoMerge           *bool   `json:"allow_auto_merge,omitempty"`
	AllowMergeCommit         *bool   `json:"allow_merge_commit,omitempty"`
	AllowRebaseMerge         *bool   `json:"allow_rebase_merge,omitempty"`
	AllowSquashMerge         *bool   `json:"allow_squash_merge,omitempty"`
	MergeCommitTitle         *string `json:"merge_commit_title,omitempty"`
	MergeCommitMessage       *string `json:"merge_commit_message,omitempty"`
	SquashMergeCommitTitle   *string `json:"squash_merge_commit_title,omitempty"`
	SquashMergeCommitMessage *string `json:"squash_merge_commit_message,omitempty"`
	Archived                 *bool   `json:"archived,omitempty"`
}

// Empty reports whether the update contains no managed changes.
func (u RepositoryUpdate) Empty() bool {
	return u.Visibility == nil &&
		u.Description == nil &&
		u.Homepage == nil &&
		u.HasIssues == nil &&
		u.HasProjects == nil &&
		u.HasWiki == nil &&
		u.HasDiscussions == nil &&
		u.DeleteBranchOnMerge == nil &&
		u.IsTemplate == nil &&
		u.AllowAutoMerge == nil &&
		u.AllowMergeCommit == nil &&
		u.AllowRebaseMerge == nil &&
		u.AllowSquashMerge == nil &&
		u.MergeCommitTitle == nil &&
		u.MergeCommitMessage == nil &&
		u.SquashMergeCommitTitle == nil &&
		u.SquashMergeCommitMessage == nil &&
		u.Archived == nil
}

// RepositoryClientFactory creates clients from provider credentials.
type RepositoryClientFactory interface {
	NewRepositoryClient(token, baseURL string) (RepositoryClient, error)
}

// RESTClientFactory creates REST-backed GitHub clients.
// Reusing one factory shares its HTTP transport and global rate-limit gate.
type RESTClientFactory struct {
	HTTPClient *http.Client
}

// NewRepositoryClient creates a REST-backed repository client.
func (f RESTClientFactory) NewRepositoryClient(token, baseURL string) (RepositoryClient, error) {
	return NewRESTClientWithHTTPClient(token, baseURL, f.HTTPClient)
}

// RepositoryClient defines the GitHub repository operations used by the controller.
type RepositoryClient interface {
	GetRepository(ctx context.Context, organization, name string) (*Repository, error)
	CreateRepository(
		ctx context.Context,
		organization string,
		input RepositoryCreate,
	) (*Repository, error)
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
	GetRepositoryVulnerabilityAlerts(
		ctx context.Context,
		organization string,
		name string,
	) (bool, error)
	SetRepositoryVulnerabilityAlerts(
		ctx context.Context,
		organization string,
		name string,
		enabled bool,
	) error
	ArchiveRepository(ctx context.Context, organization, name string) (*Repository, error)
	DeleteRepository(ctx context.Context, organization, name string) error
}
