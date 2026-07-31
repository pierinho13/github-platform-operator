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

// Team contains the GitHub organization team fields used by the operator.
type Team struct {
	ID          int64
	Name        string
	Slug        string
	Description string
	Privacy     string
}

// TeamCreate contains the settings used while creating a team.
type TeamCreate struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Privacy     string `json:"privacy"`
}

// TeamUpdate contains explicitly managed team settings.
type TeamUpdate struct {
	Description *string `json:"description,omitempty"`
	Privacy     *string `json:"privacy,omitempty"`
}

// Empty reports whether no team fields need updating.
func (u TeamUpdate) Empty() bool {
	return u.Description == nil && u.Privacy == nil
}

// Membership describes organization or team membership.
type Membership struct {
	State string
	Role  string
}

// OrganizationClientFactory creates clients used to manage organization resources.
type OrganizationClientFactory interface {
	NewOrganizationClient(token, baseURL string) (OrganizationClient, error)
}

// NewOrganizationClient creates a REST-backed organization client.
func (f RESTClientFactory) NewOrganizationClient(
	token string,
	baseURL string,
) (OrganizationClient, error) {
	return NewRESTClientWithHTTPClient(token, baseURL, f.HTTPClient)
}

// OrganizationClient defines GitHub organization membership and team operations.
type OrganizationClient interface {
	ListTeams(ctx context.Context, organization string) ([]Team, error)
	GetTeam(ctx context.Context, organization, teamSlug string) (*Team, error)
	CreateTeam(ctx context.Context, organization string, input TeamCreate) (*Team, error)
	UpdateTeam(
		ctx context.Context,
		organization string,
		teamSlug string,
		update TeamUpdate,
	) (*Team, error)
	DeleteTeam(ctx context.Context, organization, teamSlug string) error

	GetTeamMembership(
		ctx context.Context,
		organization string,
		teamSlug string,
		username string,
	) (*Membership, error)
	SetTeamMembership(
		ctx context.Context,
		organization string,
		teamSlug string,
		username string,
		role string,
	) (*Membership, error)
	RemoveTeamMembership(
		ctx context.Context,
		organization string,
		teamSlug string,
		username string,
	) error

	GetOrganizationMembership(
		ctx context.Context,
		organization string,
		username string,
	) (*Membership, error)
	SetOrganizationMembership(
		ctx context.Context,
		organization string,
		username string,
		role string,
	) (*Membership, error)
	RemoveOrganizationMembership(
		ctx context.Context,
		organization string,
		username string,
	) error
}
