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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultRequestTimeout = 30 * time.Second
	maxResponseBodySize   = 1 << 20

	actionsVisibilitySelected = "selected"
	repositoryPermissionPull  = "pull"
	repositoryPermissionPush  = "push"
	repositoryInvitationRead  = "read"
	repositoryInvitationWrite = "write"
)

// RESTClient is a minimal GitHub REST API client.
type RESTClient struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

// APIError describes an unsuccessful response from the GitHub API.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("GitHub API returned status %d: %s", e.StatusCode, e.Body)
}

// NewRESTClient creates a GitHub REST API client.
func NewRESTClient(token, baseURL string) (*RESTClient, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("GitHub token must not be empty")
	}

	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse GitHub API URL: %w", err)
	}
	if parsedURL.Scheme != "https" && parsedURL.Scheme != "http" {
		return nil, fmt.Errorf("GitHub API URL must use http or https")
	}
	if parsedURL.Host == "" {
		return nil, fmt.Errorf("GitHub API URL must include a host")
	}

	return &RESTClient{
		token:   token,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: defaultRequestTimeout,
		},
	}, nil
}

// GetRepository returns a GitHub repository.
func (c *RESTClient) GetRepository(
	ctx context.Context,
	organization string,
	name string,
) (*Repository, error) {
	endpoint := c.repositoryEndpoint(organization, name)

	response, err := c.do(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(response.Body)

	if response.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, decodeAPIError(response)
	}

	return decodeRepository(response.Body)
}

// CreateRepository creates a repository in a GitHub organization.
func (c *RESTClient) CreateRepository(
	ctx context.Context,
	organization string,
	name string,
	private bool,
) (*Repository, error) {
	requestBody, err := json.Marshal(struct {
		Name    string `json:"name"`
		Private bool   `json:"private"`
	}{
		Name:    name,
		Private: private,
	})
	if err != nil {
		return nil, fmt.Errorf("encode create repository request: %w", err)
	}

	endpoint := fmt.Sprintf(
		"%s/orgs/%s/repos",
		c.baseURL,
		url.PathEscape(organization),
	)

	response, err := c.do(ctx, http.MethodPost, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(response.Body)

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, decodeAPIError(response)
	}

	return decodeRepository(response.Body)
}

// UpdateRepository changes the explicitly managed repository settings.
func (c *RESTClient) UpdateRepository(
	ctx context.Context,
	organization string,
	name string,
	update RepositoryUpdate,
) (*Repository, error) {
	requestBody, err := json.Marshal(update)
	if err != nil {
		return nil, fmt.Errorf("encode update repository request: %w", err)
	}

	response, err := c.do(
		ctx,
		http.MethodPatch,
		c.repositoryEndpoint(organization, name),
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(response.Body)

	if response.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, decodeAPIError(response)
	}

	return decodeRepository(response.Body)
}

// ReplaceRepositoryTopics replaces the complete repository topic set.
func (c *RESTClient) ReplaceRepositoryTopics(
	ctx context.Context,
	organization string,
	name string,
	topics []string,
) ([]string, error) {
	requestBody, err := json.Marshal(struct {
		Names []string `json:"names"`
	}{
		Names: topics,
	})
	if err != nil {
		return nil, fmt.Errorf("encode repository topics request: %w", err)
	}

	response, err := c.do(
		ctx,
		http.MethodPut,
		c.repositoryTopicsEndpoint(organization, name),
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(response.Body)

	if response.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, decodeAPIError(response)
	}

	var payload struct {
		Names []string `json:"names"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxResponseBodySize)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode repository topics response: %w", err)
	}

	return payload.Names, nil
}

// DeleteRepository deletes an existing GitHub repository.
func (c *RESTClient) DeleteRepository(
	ctx context.Context,
	organization string,
	name string,
) error {
	response, err := c.do(
		ctx,
		http.MethodDelete,
		c.repositoryEndpoint(organization, name),
		nil,
	)
	if err != nil {
		return err
	}
	defer closeResponseBody(response.Body)

	if response.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return decodeAPIError(response)
	}

	return nil
}

// GetTeamRepositoryPermission returns the standard repository permission granted by a team.
func (c *RESTClient) GetTeamRepositoryPermission(
	ctx context.Context,
	organization string,
	teamSlug string,
	repositoryOwner string,
	repositoryName string,
) (string, error) {
	response, err := c.doWithAccept(
		ctx,
		http.MethodGet,
		c.teamRepositoryEndpoint(organization, teamSlug, repositoryOwner, repositoryName),
		nil,
		"application/vnd.github.v3.repository+json",
	)
	if err != nil {
		return "", err
	}
	defer closeResponseBody(response.Body)

	if response.StatusCode == http.StatusNotFound {
		return "", ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", decodeAPIError(response)
	}
	if response.StatusCode == http.StatusNoContent {
		return "", errors.New("GitHub team permission response did not include repository permissions")
	}

	return decodeRepositoryPermission(response.Body)
}

// SetTeamRepositoryPermission adds a repository to a team or updates its permission.
func (c *RESTClient) SetTeamRepositoryPermission(
	ctx context.Context,
	organization string,
	teamSlug string,
	repositoryOwner string,
	repositoryName string,
	permission string,
) error {
	requestBody, err := json.Marshal(struct {
		Permission string `json:"permission"`
	}{
		Permission: permission,
	})
	if err != nil {
		return fmt.Errorf("encode team repository permission request: %w", err)
	}

	response, err := c.do(
		ctx,
		http.MethodPut,
		c.teamRepositoryEndpoint(organization, teamSlug, repositoryOwner, repositoryName),
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return err
	}
	defer closeResponseBody(response.Body)

	if response.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return decodeAPIError(response)
	}

	return nil
}

// RemoveTeamRepository removes a repository from a team.
func (c *RESTClient) RemoveTeamRepository(
	ctx context.Context,
	organization string,
	teamSlug string,
	repositoryOwner string,
	repositoryName string,
) error {
	response, err := c.do(
		ctx,
		http.MethodDelete,
		c.teamRepositoryEndpoint(organization, teamSlug, repositoryOwner, repositoryName),
		nil,
	)
	if err != nil {
		return err
	}
	defer closeResponseBody(response.Body)

	if response.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return decodeAPIError(response)
	}

	return nil
}

// GetCollaboratorAccess returns direct access or a pending repository invitation.
func (c *RESTClient) GetCollaboratorAccess(
	ctx context.Context,
	repositoryOwner string,
	repositoryName string,
	username string,
) (*CollaboratorAccess, error) {
	directAccess, err := c.findDirectCollaborator(
		ctx,
		repositoryOwner,
		repositoryName,
		username,
	)
	if err != nil {
		return nil, err
	}
	if directAccess != nil {
		return directAccess, nil
	}

	invitation, err := c.findRepositoryInvitation(ctx, repositoryOwner, repositoryName, username)
	if err != nil {
		return nil, err
	}
	if invitation == nil {
		return nil, ErrNotFound
	}

	return invitation, nil
}

// SetCollaboratorPermission grants direct access or creates a repository invitation.
func (c *RESTClient) SetCollaboratorPermission(
	ctx context.Context,
	repositoryOwner string,
	repositoryName string,
	username string,
	permission string,
) (*CollaboratorAccess, error) {
	requestBody, err := json.Marshal(struct {
		Permission string `json:"permission"`
	}{
		Permission: permission,
	})
	if err != nil {
		return nil, fmt.Errorf("encode collaborator permission request: %w", err)
	}

	response, err := c.do(
		ctx,
		http.MethodPut,
		c.collaboratorEndpoint(repositoryOwner, repositoryName, username),
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(response.Body)

	if response.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, decodeAPIError(response)
	}

	if response.StatusCode == http.StatusCreated {
		invitation, err := decodeRepositoryInvitation(response.Body)
		if err != nil {
			return nil, err
		}
		invitation.Permission = permission
		return invitation, nil
	}

	return &CollaboratorAccess{Permission: permission}, nil
}

// UpdateRepositoryInvitation changes the permission on a pending invitation.
func (c *RESTClient) UpdateRepositoryInvitation(
	ctx context.Context,
	repositoryOwner string,
	repositoryName string,
	invitationID int64,
	permission string,
) (*CollaboratorAccess, error) {
	requestBody, err := json.Marshal(struct {
		Permissions string `json:"permissions"`
	}{
		Permissions: collaboratorPermissionToInvitationPermission(permission),
	})
	if err != nil {
		return nil, fmt.Errorf("encode repository invitation request: %w", err)
	}

	response, err := c.do(
		ctx,
		http.MethodPatch,
		c.repositoryInvitationEndpoint(repositoryOwner, repositoryName, invitationID),
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(response.Body)

	if response.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, decodeAPIError(response)
	}

	invitation, err := decodeRepositoryInvitation(response.Body)
	if err != nil {
		return nil, err
	}
	invitation.Permission = permission
	return invitation, nil
}

// RemoveCollaboratorAccess revokes direct access or deletes a pending invitation.
func (c *RESTClient) RemoveCollaboratorAccess(
	ctx context.Context,
	repositoryOwner string,
	repositoryName string,
	username string,
	invitationID int64,
) error {
	endpoint := c.collaboratorEndpoint(repositoryOwner, repositoryName, username)
	if invitationID != 0 {
		endpoint = c.repositoryInvitationEndpoint(repositoryOwner, repositoryName, invitationID)
	}

	response, err := c.do(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	defer closeResponseBody(response.Body)

	if response.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return decodeAPIError(response)
	}

	return nil
}

func (c *RESTClient) repositoryEndpoint(organization, name string) string {
	return fmt.Sprintf(
		"%s/repos/%s/%s",
		c.baseURL,
		url.PathEscape(organization),
		url.PathEscape(name),
	)
}

func (c *RESTClient) repositoryTopicsEndpoint(organization, name string) string {
	return c.repositoryEndpoint(organization, name) + "/topics"
}

func (c *RESTClient) do(
	ctx context.Context,
	method string,
	endpoint string,
	body io.Reader,
) (*http.Response, error) {
	return c.doWithAccept(ctx, method, endpoint, body, "application/vnd.github+json")
}

func (c *RESTClient) doWithAccept(
	ctx context.Context,
	method string,
	endpoint string,
	body io.Reader,
	accept string,
) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("create GitHub API request: %w", err)
	}

	request.Header.Set("Accept", accept)
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "github-platform-operator")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("execute GitHub API request: %w", err)
	}

	return response, nil
}

func (c *RESTClient) teamRepositoryEndpoint(
	organization string,
	teamSlug string,
	repositoryOwner string,
	repositoryName string,
) string {
	return fmt.Sprintf(
		"%s/orgs/%s/teams/%s/repos/%s/%s",
		c.baseURL,
		url.PathEscape(organization),
		url.PathEscape(teamSlug),
		url.PathEscape(repositoryOwner),
		url.PathEscape(repositoryName),
	)
}

func (c *RESTClient) collaboratorEndpoint(
	repositoryOwner string,
	repositoryName string,
	username string,
) string {
	return fmt.Sprintf(
		"%s/repos/%s/%s/collaborators/%s",
		c.baseURL,
		url.PathEscape(repositoryOwner),
		url.PathEscape(repositoryName),
		url.PathEscape(username),
	)
}

func (c *RESTClient) directCollaboratorsEndpoint(
	repositoryOwner string,
	repositoryName string,
	page int,
) string {
	return fmt.Sprintf(
		"%s/repos/%s/%s/collaborators?affiliation=direct&per_page=100&page=%d",
		c.baseURL,
		url.PathEscape(repositoryOwner),
		url.PathEscape(repositoryName),
		page,
	)
}

func (c *RESTClient) repositoryInvitationsEndpoint(
	repositoryOwner string,
	repositoryName string,
) string {
	return fmt.Sprintf(
		"%s/repos/%s/%s/invitations?per_page=100",
		c.baseURL,
		url.PathEscape(repositoryOwner),
		url.PathEscape(repositoryName),
	)
}

func (c *RESTClient) repositoryInvitationEndpoint(
	repositoryOwner string,
	repositoryName string,
	invitationID int64,
) string {
	return fmt.Sprintf(
		"%s/repos/%s/%s/invitations/%d",
		c.baseURL,
		url.PathEscape(repositoryOwner),
		url.PathEscape(repositoryName),
		invitationID,
	)
}

func (c *RESTClient) findDirectCollaborator(
	ctx context.Context,
	repositoryOwner string,
	repositoryName string,
	username string,
) (*CollaboratorAccess, error) {
	for page := 1; ; page++ {
		response, err := c.do(
			ctx,
			http.MethodGet,
			c.directCollaboratorsEndpoint(repositoryOwner, repositoryName, page),
			nil,
		)
		if err != nil {
			return nil, err
		}

		if response.StatusCode == http.StatusNotFound {
			closeResponseBody(response.Body)
			return nil, ErrNotFound
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			apiErr := decodeAPIError(response)
			closeResponseBody(response.Body)
			return nil, apiErr
		}

		var payload []struct {
			Login       string `json:"login"`
			RoleName    string `json:"role_name"`
			Permissions struct {
				Pull     bool `json:"pull"`
				Triage   bool `json:"triage"`
				Push     bool `json:"push"`
				Maintain bool `json:"maintain"`
				Admin    bool `json:"admin"`
			} `json:"permissions"`
		}
		decodeErr := json.NewDecoder(
			io.LimitReader(response.Body, maxResponseBodySize),
		).Decode(&payload)
		closeResponseBody(response.Body)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode direct collaborators response: %w", decodeErr)
		}

		for i := range payload {
			if strings.EqualFold(payload[i].Login, username) {
				permission, err := permissionFromGitHubFields(
					payload[i].RoleName,
					payload[i].Permissions.Pull,
					payload[i].Permissions.Triage,
					payload[i].Permissions.Push,
					payload[i].Permissions.Maintain,
					payload[i].Permissions.Admin,
				)
				if err != nil {
					return nil, err
				}
				return &CollaboratorAccess{Permission: permission}, nil
			}
		}

		if len(payload) < 100 {
			return nil, nil
		}
	}
}

func (c *RESTClient) findRepositoryInvitation(
	ctx context.Context,
	repositoryOwner string,
	repositoryName string,
	username string,
) (*CollaboratorAccess, error) {
	response, err := c.do(
		ctx,
		http.MethodGet,
		c.repositoryInvitationsEndpoint(repositoryOwner, repositoryName),
		nil,
	)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(response.Body)

	if response.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, decodeAPIError(response)
	}

	var payload []struct {
		ID          int64  `json:"id"`
		Permissions string `json:"permissions"`
		Invitee     struct {
			Login string `json:"login"`
		} `json:"invitee"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxResponseBodySize)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode repository invitations response: %w", err)
	}

	for i := range payload {
		if strings.EqualFold(payload[i].Invitee.Login, username) {
			return &CollaboratorAccess{
				Permission:        invitationPermissionToCollaboratorPermission(payload[i].Permissions),
				InvitationPending: true,
				InvitationID:      payload[i].ID,
			}, nil
		}
	}

	return nil, nil
}

func decodeRepositoryPermission(body io.Reader) (string, error) {
	var payload struct {
		RoleName    string `json:"role_name"`
		Permissions struct {
			Pull     bool `json:"pull"`
			Triage   bool `json:"triage"`
			Push     bool `json:"push"`
			Maintain bool `json:"maintain"`
			Admin    bool `json:"admin"`
		} `json:"permissions"`
	}

	if err := json.NewDecoder(io.LimitReader(body, maxResponseBodySize)).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode team repository permission response: %w", err)
	}

	return permissionFromGitHubFields(
		payload.RoleName,
		payload.Permissions.Pull,
		payload.Permissions.Triage,
		payload.Permissions.Push,
		payload.Permissions.Maintain,
		payload.Permissions.Admin,
	)
}

func permissionFromGitHubFields(
	roleName string,
	pull bool,
	triage bool,
	push bool,
	maintain bool,
	admin bool,
) (string, error) {
	switch {
	case admin:
		return "admin", nil
	case maintain:
		return "maintain", nil
	case push:
		return repositoryPermissionPush, nil
	case triage:
		return "triage", nil
	case pull:
		return repositoryPermissionPull, nil
	}

	switch roleName {
	case "admin", "maintain", "triage", repositoryPermissionPush, repositoryPermissionPull:
		return roleName, nil
	case repositoryInvitationWrite:
		return repositoryPermissionPush, nil
	case repositoryInvitationRead:
		return repositoryPermissionPull, nil
	default:
		return "", fmt.Errorf("GitHub response did not contain a supported repository permission")
	}
}

func decodeRepositoryInvitation(body io.Reader) (*CollaboratorAccess, error) {
	var payload struct {
		ID          int64  `json:"id"`
		Permissions string `json:"permissions"`
	}

	if err := json.NewDecoder(io.LimitReader(body, maxResponseBodySize)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode repository invitation response: %w", err)
	}

	return &CollaboratorAccess{
		Permission:        invitationPermissionToCollaboratorPermission(payload.Permissions),
		InvitationPending: true,
		InvitationID:      payload.ID,
	}, nil
}

func collaboratorPermissionToInvitationPermission(permission string) string {
	switch permission {
	case repositoryPermissionPull:
		return repositoryInvitationRead
	case repositoryPermissionPush:
		return repositoryInvitationWrite
	default:
		return permission
	}
}

func invitationPermissionToCollaboratorPermission(permission string) string {
	switch permission {
	case repositoryInvitationRead:
		return repositoryPermissionPull
	case repositoryInvitationWrite:
		return repositoryPermissionPush
	default:
		return permission
	}
}

func closeResponseBody(body io.ReadCloser) {
	_ = body.Close()
}

func decodeRepository(body io.Reader) (*Repository, error) {
	var payload struct {
		ID             int64    `json:"id"`
		HTMLURL        string   `json:"html_url"`
		Visibility     string   `json:"visibility"`
		Private        bool     `json:"private"`
		Description    *string  `json:"description"`
		Homepage       *string  `json:"homepage"`
		Topics         []string `json:"topics"`
		HasIssues      bool     `json:"has_issues"`
		HasProjects    bool     `json:"has_projects"`
		HasWiki        bool     `json:"has_wiki"`
		HasDiscussions bool     `json:"has_discussions"`
	}

	if err := json.NewDecoder(io.LimitReader(body, maxResponseBodySize)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode GitHub repository response: %w", err)
	}

	visibility := payload.Visibility
	if visibility == "" {
		visibility = "public"
		if payload.Private {
			visibility = "private"
		}
	}

	description := ""
	if payload.Description != nil {
		description = *payload.Description
	}
	homepage := ""
	if payload.Homepage != nil {
		homepage = *payload.Homepage
	}

	return &Repository{
		ID:             payload.ID,
		HTMLURL:        payload.HTMLURL,
		Visibility:     visibility,
		Description:    description,
		Homepage:       homepage,
		Topics:         payload.Topics,
		HasIssues:      payload.HasIssues,
		HasProjects:    payload.HasProjects,
		HasWiki:        payload.HasWiki,
		HasDiscussions: payload.HasDiscussions,
	}, nil
}

func decodeAPIError(response *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBodySize))
	if err != nil {
		return fmt.Errorf("read GitHub API error response: %w", err)
	}

	return &APIError{
		StatusCode: response.StatusCode,
		Body:       strings.TrimSpace(string(body)),
	}
}
