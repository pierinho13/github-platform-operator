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
	testRulesetName           = "protect-main"
	testRulesetCollectionPath = "/repos/k8sready/example/rulesets"
	testRulesetItemPath       = testRulesetCollectionPath + "/42"
)

func TestRepositoryRulesetEndpoints(t *testing.T) {
	t.Parallel()

	server := newRepositoryRulesetTestServer(t)
	defer server.Close()

	client, err := NewRESTClient("token", server.URL)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	summaries, err := client.ListRepositoryRulesets(context.Background(), "k8sready", "example")
	if err != nil {
		t.Fatalf("list rulesets: %v", err)
	}
	if len(summaries) != 1 || summaries[0].ID != 42 || summaries[0].Name != testRulesetName {
		t.Fatalf("unexpected summaries %#v", summaries)
	}

	input := RepositoryRulesetUpsert{
		Name:        testRulesetName,
		Target:      "branch",
		Enforcement: "active",
		Conditions: &RulesetConditions{RefName: &RulesetRefNameCondition{
			Include: []string{"~DEFAULT_BRANCH"},
			Exclude: []string{},
		}},
		Rules: []RulesetRule{
			{Type: "deletion"},
			{
				Type: "pull_request",
				Parameters: json.RawMessage(`{
					"required_approving_review_count":1,
					"dismiss_stale_reviews_on_push":true,
					"require_code_owner_review":true,
					"require_last_push_approval":false,
					"required_review_thread_resolution":true,
					"allowed_merge_methods":["squash"]
				}`),
			},
		},
	}

	created, err := client.CreateRepositoryRuleset(context.Background(), "k8sready", "example", input)
	if err != nil {
		t.Fatalf("create ruleset: %v", err)
	}
	if created.ID != 42 || created.HTMLURL == "" {
		t.Fatalf("unexpected created ruleset %#v", created)
	}

	observed, err := client.GetRepositoryRuleset(context.Background(), "k8sready", "example", 42)
	if err != nil {
		t.Fatalf("get ruleset: %v", err)
	}
	if observed.Name != testRulesetName || len(observed.Rules) != 2 {
		t.Fatalf("unexpected observed ruleset %#v", observed)
	}

	input.Enforcement = "disabled"
	updated, err := client.UpdateRepositoryRuleset(
		context.Background(), "k8sready", "example", 42, input,
	)
	if err != nil {
		t.Fatalf("update ruleset: %v", err)
	}
	if updated.Enforcement != "disabled" {
		t.Fatalf("unexpected updated enforcement %q", updated.Enforcement)
	}

	if err := client.DeleteRepositoryRuleset(
		context.Background(), "k8sready", "example", 42,
	); err != nil {
		t.Fatalf("delete ruleset: %v", err)
	}
}

func newRepositoryRulesetTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	requestNumber := 0
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestNumber++
		if request.Header.Get("X-GitHub-Api-Version") != githubAPIVersion {
			t.Fatalf("unexpected API version %q", request.Header.Get("X-GitHub-Api-Version"))
		}
		handleRepositoryRulesetTestRequest(t, requestNumber, writer, request)
	}))
}

func handleRepositoryRulesetTestRequest(
	t *testing.T,
	requestNumber int,
	writer http.ResponseWriter,
	request *http.Request,
) {
	t.Helper()

	switch requestNumber {
	case 1:
		assertRulesetHTTPTestRequest(t, request, http.MethodGet, testRulesetCollectionPath)
		if request.URL.Query().Get("includes_parents") != "false" {
			t.Fatalf("unexpected list query %s", request.URL.RawQuery)
		}
		_, _ = writer.Write([]byte(`[{
			"id":42,
			"name":"protect-main",
			"source_type":"Repository",
			"source":"k8sready/example",
			"enforcement":"active"
		}]`))
	case 2:
		assertRulesetHTTPTestRequest(t, request, http.MethodPost, testRulesetCollectionPath)
		assertRulesetRequest(t, request, "active")
		writer.WriteHeader(http.StatusCreated)
		writeRulesetResponse(writer, "active")
	case 3:
		assertRulesetHTTPTestRequest(t, request, http.MethodGet, testRulesetItemPath)
		writeRulesetResponse(writer, "active")
	case 4:
		assertRulesetHTTPTestRequest(t, request, http.MethodPut, testRulesetItemPath)
		assertRulesetRequest(t, request, "disabled")
		writeRulesetResponse(writer, "disabled")
	case 5:
		assertRulesetHTTPTestRequest(t, request, http.MethodDelete, testRulesetItemPath)
		writer.WriteHeader(http.StatusNoContent)
	default:
		t.Fatalf("unexpected extra request %s %s", request.Method, request.URL.Path)
	}
}

func assertRulesetHTTPTestRequest(
	t *testing.T,
	request *http.Request,
	method string,
	path string,
) {
	t.Helper()
	if request.Method != method || request.URL.Path != path {
		t.Fatalf("unexpected request %s %s", request.Method, request.URL.String())
	}
}

func assertRulesetRequest(t *testing.T, request *http.Request, enforcement string) {
	t.Helper()

	var input RepositoryRulesetUpsert
	if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
		t.Fatalf("decode ruleset request: %v", err)
	}
	if input.Name != testRulesetName || input.Target != "branch" || input.Enforcement != enforcement {
		t.Fatalf("unexpected ruleset request %#v", input)
	}
	if input.Conditions == nil || input.Conditions.RefName == nil ||
		len(input.Conditions.RefName.Include) != 1 ||
		input.Conditions.RefName.Include[0] != "~DEFAULT_BRANCH" {
		t.Fatalf("unexpected ruleset conditions %#v", input.Conditions)
	}
	if len(input.Rules) != 2 || input.Rules[1].Type != "pull_request" {
		t.Fatalf("unexpected ruleset rules %#v", input.Rules)
	}
}

func writeRulesetResponse(writer http.ResponseWriter, enforcement string) {
	_, _ = writer.Write([]byte(`{
		"id":42,
		"name":"` + testRulesetName + `",
		"target":"branch",
		"source_type":"Repository",
		"source":"k8sready/example",
		"enforcement":"` + enforcement + `",
		"conditions":{"ref_name":{"include":["~DEFAULT_BRANCH"],"exclude":[]}},
		"rules":[
			{"type":"deletion"},
			{"type":"pull_request","parameters":{"required_approving_review_count":1}}
		],
		"_links":{"html":{"href":"https://github.com/k8sready/example/rules/42"}}
	}`))
}
