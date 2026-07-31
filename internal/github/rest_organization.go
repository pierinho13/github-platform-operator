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
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// ListTeams returns every team visible in an organization.
func (c *RESTClient) ListTeams(ctx context.Context, organization string) ([]Team, error) {
	teams := make([]Team, 0)
	for page := 1; ; page++ {
		endpoint := fmt.Sprintf(
			"%s/orgs/%s/teams?per_page=100&page=%d",
			c.baseURL,
			url.PathEscape(organization),
			page,
		)
		response, err := c.do(ctx, http.MethodGet, endpoint, nil)
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

		var payload []teamPayload
		decodeErr := json.NewDecoder(
			io.LimitReader(response.Body, maxResponseBodySize),
		).Decode(&payload)
		closeResponseBody(response.Body)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode GitHub teams response: %w", decodeErr)
		}
		for i := range payload {
			teams = append(teams, payload[i].team())
		}
		if len(payload) < 100 {
			return teams, nil
		}
	}
}

// GetTeam returns one organization team by slug.
func (c *RESTClient) GetTeam(
	ctx context.Context,
	organization string,
	teamSlug string,
) (*Team, error) {
	response, err := c.do(ctx, http.MethodGet, c.teamEndpoint(organization, teamSlug), nil)
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
	return decodeTeam(response.Body)
}

// CreateTeam creates an organization team.
func (c *RESTClient) CreateTeam(
	ctx context.Context,
	organization string,
	input TeamCreate,
) (*Team, error) {
	requestBody, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode create team request: %w", err)
	}
	endpoint := fmt.Sprintf("%s/orgs/%s/teams", c.baseURL, url.PathEscape(organization))
	response, err := c.do(ctx, http.MethodPost, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(response.Body)
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, decodeAPIError(response)
	}
	return decodeTeam(response.Body)
}

// UpdateTeam changes explicitly managed team settings.
func (c *RESTClient) UpdateTeam(
	ctx context.Context,
	organization string,
	teamSlug string,
	update TeamUpdate,
) (*Team, error) {
	requestBody, err := json.Marshal(update)
	if err != nil {
		return nil, fmt.Errorf("encode update team request: %w", err)
	}
	response, err := c.do(
		ctx,
		http.MethodPatch,
		c.teamEndpoint(organization, teamSlug),
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
	return decodeTeam(response.Body)
}

// DeleteTeam deletes an organization team.
func (c *RESTClient) DeleteTeam(
	ctx context.Context,
	organization string,
	teamSlug string,
) error {
	response, err := c.do(ctx, http.MethodDelete, c.teamEndpoint(organization, teamSlug), nil)
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

// GetTeamMembership returns a user's membership in a team.
func (c *RESTClient) GetTeamMembership(
	ctx context.Context,
	organization string,
	teamSlug string,
	username string,
) (*Membership, error) {
	return c.getMembership(ctx, c.teamMembershipEndpoint(organization, teamSlug, username))
}

// SetTeamMembership adds a user to a team or updates their role.
func (c *RESTClient) SetTeamMembership(
	ctx context.Context,
	organization string,
	teamSlug string,
	username string,
	role string,
) (*Membership, error) {
	return c.setMembership(
		ctx,
		c.teamMembershipEndpoint(organization, teamSlug, username),
		role,
	)
}

// RemoveTeamMembership removes a user from a team.
func (c *RESTClient) RemoveTeamMembership(
	ctx context.Context,
	organization string,
	teamSlug string,
	username string,
) error {
	return c.removeMembership(ctx, c.teamMembershipEndpoint(organization, teamSlug, username))
}

// GetOrganizationMembership returns direct organization membership for a user.
func (c *RESTClient) GetOrganizationMembership(
	ctx context.Context,
	organization string,
	username string,
) (*Membership, error) {
	return c.getMembership(ctx, c.organizationMembershipEndpoint(organization, username))
}

// SetOrganizationMembership invites a user or updates their organization role.
func (c *RESTClient) SetOrganizationMembership(
	ctx context.Context,
	organization string,
	username string,
	role string,
) (*Membership, error) {
	return c.setMembership(ctx, c.organizationMembershipEndpoint(organization, username), role)
}

// RemoveOrganizationMembership removes a user from the organization.
func (c *RESTClient) RemoveOrganizationMembership(
	ctx context.Context,
	organization string,
	username string,
) error {
	return c.removeMembership(ctx, c.organizationMembershipEndpoint(organization, username))
}

func (c *RESTClient) getMembership(ctx context.Context, endpoint string) (*Membership, error) {
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
	return decodeMembership(response.Body)
}

func (c *RESTClient) setMembership(
	ctx context.Context,
	endpoint string,
	role string,
) (*Membership, error) {
	requestBody, err := json.Marshal(struct {
		Role string `json:"role"`
	}{Role: role})
	if err != nil {
		return nil, fmt.Errorf("encode membership request: %w", err)
	}
	response, err := c.do(ctx, http.MethodPut, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(response.Body)
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, decodeAPIError(response)
	}
	return decodeMembership(response.Body)
}

func (c *RESTClient) removeMembership(ctx context.Context, endpoint string) error {
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

func (c *RESTClient) teamEndpoint(organization, teamSlug string) string {
	return fmt.Sprintf(
		"%s/orgs/%s/teams/%s",
		c.baseURL,
		url.PathEscape(organization),
		url.PathEscape(teamSlug),
	)
}

func (c *RESTClient) teamMembershipEndpoint(
	organization string,
	teamSlug string,
	username string,
) string {
	return fmt.Sprintf(
		"%s/memberships/%s",
		c.teamEndpoint(organization, teamSlug),
		url.PathEscape(username),
	)
}

func (c *RESTClient) organizationMembershipEndpoint(organization, username string) string {
	return fmt.Sprintf(
		"%s/orgs/%s/memberships/%s",
		c.baseURL,
		url.PathEscape(organization),
		url.PathEscape(username),
	)
}

type teamPayload struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Description *string `json:"description"`
	Privacy     string  `json:"privacy"`
}

func (p teamPayload) team() Team {
	description := ""
	if p.Description != nil {
		description = *p.Description
	}
	return Team{
		ID:          p.ID,
		Name:        p.Name,
		Slug:        p.Slug,
		Description: description,
		Privacy:     p.Privacy,
	}
}

func decodeTeam(body io.Reader) (*Team, error) {
	var payload teamPayload
	if err := json.NewDecoder(io.LimitReader(body, maxResponseBodySize)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode GitHub team response: %w", err)
	}
	team := payload.team()
	return &team, nil
}

func decodeMembership(body io.Reader) (*Membership, error) {
	var payload struct {
		State string `json:"state"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(io.LimitReader(body, maxResponseBodySize)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode GitHub membership response: %w", err)
	}
	return &Membership{State: payload.State, Role: payload.Role}, nil
}
