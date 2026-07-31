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
	"net/http/httptest"
	"testing"
)

const (
	rulesetActorTestOrganization = "k8sready"
	rulesetActorTestTeamSlug     = "platform"
	rulesetActorTestUsername     = "octocat"
	rulesetActorTestToken        = "token"
	rulesetActorTeamPath         = "/orgs/k8sready/teams/platform"
	rulesetActorUserPath         = "/users/octocat"
	rulesetActorCreateClientErr  = "create client: %v"
)

func TestRepositoryRulesetActorResolutionEndpoints(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Fatalf("unexpected method %s", request.Method)
		}
		if request.Header.Get("X-GitHub-Api-Version") != githubAPIVersion {
			t.Fatalf("unexpected API version %q", request.Header.Get("X-GitHub-Api-Version"))
		}

		switch request.URL.Path {
		case rulesetActorTeamPath:
			_, _ = writer.Write([]byte(`{"id":1001,"slug":"platform"}`))
		case rulesetActorUserPath:
			_, _ = writer.Write([]byte(`{"id":1002,"login":"octocat"}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewRESTClient(rulesetActorTestToken, server.URL)
	if err != nil {
		t.Fatalf(rulesetActorCreateClientErr, err)
	}

	teamID, err := client.GetTeamIDBySlug(
		context.Background(),
		rulesetActorTestOrganization,
		rulesetActorTestTeamSlug,
	)
	if err != nil {
		t.Fatalf("resolve team ID: %v", err)
	}
	if teamID != 1001 {
		t.Fatalf("unexpected team ID %d", teamID)
	}

	userID, err := client.GetUserIDByUsername(context.Background(), rulesetActorTestUsername)
	if err != nil {
		t.Fatalf("resolve user ID: %v", err)
	}
	if userID != 1002 {
		t.Fatalf("unexpected user ID %d", userID)
	}
}

func TestRepositoryRulesetActorResolutionErrors(t *testing.T) {
	t.Parallel()

	t.Run("maps not found responses", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"message":"Not Found"}`))
		}))
		defer server.Close()

		client, err := NewRESTClient(rulesetActorTestToken, server.URL)
		if err != nil {
			t.Fatalf(rulesetActorCreateClientErr, err)
		}
		_, err = client.GetTeamIDBySlug(
			context.Background(),
			rulesetActorTestOrganization,
			rulesetActorTestTeamSlug,
		)
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("rejects invalid actor IDs", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte(`{"id":0}`))
		}))
		defer server.Close()

		client, err := NewRESTClient(rulesetActorTestToken, server.URL)
		if err != nil {
			t.Fatalf(rulesetActorCreateClientErr, err)
		}
		if _, err := client.GetUserIDByUsername(
			context.Background(),
			rulesetActorTestUsername,
		); err == nil {
			t.Fatal("expected invalid actor ID to be rejected")
		}
	})
}
