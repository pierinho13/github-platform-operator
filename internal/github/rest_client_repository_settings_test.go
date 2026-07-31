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
	repositorySettingsTopicKubernetes = "kubernetes"
	repositorySettingsTopicPlatform   = "platform"
	repositoryInternalVisibility      = "internal"
)

func TestUpdateRepositorySettings(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPatch {
			t.Fatalf("expected PATCH, got %s", request.Method)
		}
		if request.URL.Path != "/repos/k8sready/example-repository" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}

		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(body) != 3 {
			t.Fatalf("expected only managed fields, got %#v", body)
		}
		if body["description"] != "Platform API" {
			t.Fatalf("unexpected description %#v", body["description"])
		}
		if body["homepage"] != "" {
			t.Fatalf("expected homepage clearing, got %#v", body["homepage"])
		}
		if body["has_issues"] != false {
			t.Fatalf("unexpected has_issues %#v", body["has_issues"])
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"id": 42,
			"html_url": "https://github.com/k8sready/example-repository",
			"visibility": "private",
			"description": "Platform API",
			"homepage": null,
			"topics": ["kubernetes"],
			"has_issues": false,
			"has_projects": true,
			"has_wiki": false,
			"has_discussions": true
		}`))
	}))
	defer server.Close()

	client, err := NewRESTClient("token", server.URL)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	description := "Platform API"
	homepage := ""
	hasIssues := false
	repository, err := client.UpdateRepository(
		context.Background(),
		rulesetActorTestOrganization,
		"example-repository",
		RepositoryUpdate{
			Description: &description,
			Homepage:    &homepage,
			HasIssues:   &hasIssues,
		},
	)
	if err != nil {
		t.Fatalf("update repository: %v", err)
	}
	if repository.Description != description {
		t.Fatalf("unexpected description %q", repository.Description)
	}
	if repository.Homepage != "" {
		t.Fatalf("expected empty homepage, got %q", repository.Homepage)
	}
	if repository.HasIssues {
		t.Fatal("expected issues to be disabled")
	}
	if !repository.HasDiscussions {
		t.Fatal("expected discussions to be enabled")
	}
}

func TestReplaceRepositoryTopics(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPut {
			t.Fatalf("expected PUT, got %s", request.Method)
		}
		if request.URL.Path != "/repos/k8sready/example-repository/topics" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}

		var body struct {
			Names []string `json:"names"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(body.Names) != 2 || body.Names[0] != repositorySettingsTopicKubernetes || body.Names[1] != repositorySettingsTopicPlatform {
			t.Fatalf("unexpected topics %#v", body.Names)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"names":["kubernetes","platform"]}`))
	}))
	defer server.Close()

	client, err := NewRESTClient("token", server.URL)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	topics, err := client.ReplaceRepositoryTopics(
		context.Background(),
		rulesetActorTestOrganization,
		"example-repository",
		[]string{repositorySettingsTopicKubernetes, repositorySettingsTopicPlatform},
	)
	if err != nil {
		t.Fatalf("replace topics: %v", err)
	}
	if len(topics) != 2 || topics[0] != repositorySettingsTopicKubernetes || topics[1] != repositorySettingsTopicPlatform {
		t.Fatalf("unexpected response topics %#v", topics)
	}
}

func TestCreateRepositoryWithOptionalCreationSettings(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/orgs/k8sready/repos" {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["name"] != "example-repository" || body["visibility"] != repositoryInternalVisibility || body["auto_init"] != true {
			t.Fatalf("unexpected create body %#v", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"id":42,
			"html_url":"https://github.com/k8sready/example-repository",
			"visibility":"internal",
			"delete_branch_on_merge":true,
			"allow_auto_merge":true,
			"allow_merge_commit":false,
			"allow_rebase_merge":false,
			"allow_squash_merge":true,
			"merge_commit_title":"PR_TITLE",
			"merge_commit_message":"PR_BODY",
			"squash_merge_commit_title":"COMMIT_OR_PR_TITLE",
			"squash_merge_commit_message":"COMMIT_MESSAGES"
		}`))
	}))
	defer server.Close()

	client, err := NewRESTClient("token", server.URL)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	repository, err := client.CreateRepository(context.Background(), rulesetActorTestOrganization, RepositoryCreate{
		Name:       "example-repository",
		Visibility: repositoryInternalVisibility,
		AutoInit:   true,
	})
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if repository.Visibility != repositoryInternalVisibility || !repository.DeleteBranchOnMerge || !repository.AllowAutoMerge {
		t.Fatalf("unexpected repository %#v", repository)
	}
}

func TestCreateRepositoryFromTemplate(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/repos/templates/service/generate" {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["owner"] != rulesetActorTestOrganization || body["name"] != "generated-service" || body["include_all_branches"] != true || body["private"] != true {
			t.Fatalf("unexpected template body %#v", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":43,"html_url":"https://github.com/k8sready/generated-service","visibility":"private"}`))
	}))
	defer server.Close()

	client, err := NewRESTClient("token", server.URL)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	_, err = client.CreateRepository(context.Background(), rulesetActorTestOrganization, RepositoryCreate{
		Name:       "generated-service",
		Visibility: repositoryInternalVisibility,
		Template: &RepositoryTemplateCreate{
			Owner:              "templates",
			Repository:         "service",
			IncludeAllBranches: true,
		},
	})
	if err != nil {
		t.Fatalf("create repository from template: %v", err)
	}
}

func TestRepositoryVulnerabilityAlertsAndArchive(t *testing.T) {
	t.Parallel()

	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		methods = append(methods, request.Method+" "+request.URL.Path)
		switch {
		case request.URL.Path == "/repos/k8sready/example-repository/vulnerability-alerts" && request.Method == http.MethodGet:
			writer.WriteHeader(http.StatusNoContent)
		case request.URL.Path == "/repos/k8sready/example-repository/vulnerability-alerts" && request.Method == http.MethodDelete:
			writer.WriteHeader(http.StatusNoContent)
		case request.URL.Path == "/repos/k8sready/example-repository" && request.Method == http.MethodPatch:
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("decode archive request: %v", err)
			}
			if len(body) != 1 || body["archived"] != true {
				t.Fatalf("unexpected archive body %#v", body)
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"id":42,"html_url":"https://github.com/k8sready/example-repository","visibility":"private","archived":true}`))
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewRESTClient("token", server.URL)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	enabled, err := client.GetRepositoryVulnerabilityAlerts(context.Background(), rulesetActorTestOrganization, "example-repository")
	if err != nil || !enabled {
		t.Fatalf("get vulnerability alerts: enabled=%t err=%v", enabled, err)
	}
	if err := client.SetRepositoryVulnerabilityAlerts(context.Background(), rulesetActorTestOrganization, "example-repository", false); err != nil {
		t.Fatalf("disable vulnerability alerts: %v", err)
	}
	repository, err := client.ArchiveRepository(context.Background(), rulesetActorTestOrganization, "example-repository")
	if err != nil {
		t.Fatalf("archive repository: %v", err)
	}
	if !repository.Archived {
		t.Fatal("expected archived repository")
	}
	if len(methods) != 3 {
		t.Fatalf("unexpected calls %#v", methods)
	}
}
