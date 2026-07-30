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

const repositorySettingsTopicKubernetes = "kubernetes"

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
		"k8sready",
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
		if len(body.Names) != 2 || body.Names[0] != repositorySettingsTopicKubernetes || body.Names[1] != "platform" {
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
		"k8sready",
		"example-repository",
		[]string{repositorySettingsTopicKubernetes, "platform"},
	)
	if err != nil {
		t.Fatalf("replace topics: %v", err)
	}
	if len(topics) != 2 || topics[0] != repositorySettingsTopicKubernetes || topics[1] != "platform" {
		t.Fatalf("unexpected response topics %#v", topics)
	}
}
