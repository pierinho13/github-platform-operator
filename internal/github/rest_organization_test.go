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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	testTeamPrivacySecret     = "secret"
	testOrganizationAdminRole = "admin"
)

func TestOrganizationTeamContract(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/orgs/k8sready/teams":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`[{"id":7,"name":"Platform","slug":"platform","description":"Platform team","privacy":"closed"}]`))
		case request.Method == http.MethodPost && request.URL.Path == "/orgs/k8sready/teams":
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("decode create team request: %v", err)
			}
			if body["name"] != "Platform" || body["description"] != "Platform team" || body["privacy"] != "closed" {
				t.Fatalf("unexpected create team body %#v", body)
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"id":7,"name":"Platform","slug":"platform","description":"Platform team","privacy":"closed"}`))
		case request.Method == http.MethodPatch && request.URL.Path == "/orgs/k8sready/teams/platform":
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("decode update team request: %v", err)
			}
			if len(body) != 1 || body["privacy"] != testTeamPrivacySecret {
				t.Fatalf("unexpected update team body %#v", body)
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"id":7,"name":"Platform","slug":"platform","description":"Platform team","privacy":"` + testTeamPrivacySecret + `"}`))
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewRESTClient("token", server.URL)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	teams, err := client.ListTeams(context.Background(), "k8sready")
	if err != nil || len(teams) != 1 || teams[0].Slug != "platform" {
		t.Fatalf("list teams: teams=%#v err=%v", teams, err)
	}
	team, err := client.CreateTeam(context.Background(), "k8sready", TeamCreate{
		Name:        "Platform",
		Description: "Platform team",
		Privacy:     "closed",
	})
	if err != nil || team.ID != 7 {
		t.Fatalf("create team: team=%#v err=%v", team, err)
	}
	privacy := testTeamPrivacySecret
	team, err = client.UpdateTeam(context.Background(), "k8sready", "platform", TeamUpdate{Privacy: &privacy})
	if err != nil || team.Privacy != testTeamPrivacySecret {
		t.Fatalf("update team: team=%#v err=%v", team, err)
	}
}

func TestOrganizationAndTeamMembershipContracts(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var expectedRole string
		switch request.URL.Path {
		case "/orgs/k8sready/memberships/octocat":
			expectedRole = testOrganizationAdminRole
		case "/orgs/k8sready/teams/platform/memberships/octocat":
			expectedRole = "maintainer"
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		if request.Method != http.MethodPut {
			t.Fatalf("expected PUT, got %s", request.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode membership request: %v", err)
		}
		if body["role"] != expectedRole {
			t.Fatalf("unexpected membership body %#v", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"state":"pending","role":"` + expectedRole + `"}`))
	}))
	defer server.Close()

	client, err := NewRESTClient("token", server.URL)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	organizationMembership, err := client.SetOrganizationMembership(
		context.Background(), "k8sready", "octocat", testOrganizationAdminRole,
	)
	if err != nil || organizationMembership.State != "pending" || organizationMembership.Role != testOrganizationAdminRole {
		t.Fatalf("set organization membership: membership=%#v err=%v", organizationMembership, err)
	}
	teamMembership, err := client.SetTeamMembership(
		context.Background(), "k8sready", "platform", "octocat", "maintainer",
	)
	if err != nil || teamMembership.Role != "maintainer" {
		t.Fatalf("set team membership: membership=%#v err=%v", teamMembership, err)
	}
}
