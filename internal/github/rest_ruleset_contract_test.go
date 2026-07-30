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
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

const (
	contractRulesetTargetBranch = "branch"
	contractRulesetActive       = "active"
	contractRulesetDeletion     = "deletion"
	contractDefaultBranch       = "~DEFAULT_BRANCH"
	contractOrganization        = "k8sready"
	contractRepository          = "example"
	contractAccessToken         = "token"
	contractGitHubAPIURL        = "https://api.github.com"
	contractGitHubAppID         = "Iv1.contract-test"
	contractFarFutureExpiration = "2099-01-01T00:00:00Z"
	contractCreateClientError   = "create client: %v"
	contractBypassActorsJSON    = "bypass_actors"
	contractConditionsJSON      = "conditions"
)

func TestRepositoryRulesetCreateJSONContract(t *testing.T) {
	t.Parallel()

	t.Run("serializes empty ref-name collections as arrays instead of null", func(t *testing.T) {
		t.Parallel()

		payload := captureCreateRulesetPayload(t, RepositoryRulesetUpsert{
			Name:        testRulesetName,
			Target:      contractRulesetTargetBranch,
			Enforcement: contractRulesetActive,
			Conditions: &RulesetConditions{RefName: &RulesetRefNameCondition{
				Include: []string{contractDefaultBranch},
				Exclude: []string{},
			}},
			Rules: []RulesetRule{{Type: contractRulesetDeletion}},
		})

		conditions := requireJSONMapContractTest(t, payload, contractConditionsJSON)
		refName := requireJSONMapContractTest(t, conditions, "ref_name")
		include := requireJSONArrayContractTest(t, refName, "include")
		exclude := requireJSONArrayContractTest(t, refName, "exclude")
		if len(include) != 1 || include[0] != contractDefaultBranch {
			t.Fatalf("unexpected include payload %#v", include)
		}
		if len(exclude) != 0 {
			t.Fatalf("expected empty exclude payload, got %#v", exclude)
		}
	})

	t.Run("omits unmanaged conditions", func(t *testing.T) {
		t.Parallel()

		payload := captureCreateRulesetPayload(t, RepositoryRulesetUpsert{
			Name:        testRulesetName,
			Target:      contractRulesetTargetBranch,
			Enforcement: contractRulesetActive,
			Rules:       []RulesetRule{{Type: contractRulesetDeletion}},
		})
		if _, ok := payload[contractConditionsJSON]; ok {
			t.Fatalf("expected conditions to be omitted, got %#v", payload[contractConditionsJSON])
		}
	})

	t.Run("distinguishes omitted and explicitly empty bypass actors", func(t *testing.T) {
		t.Parallel()

		omitted := captureCreateRulesetPayload(t, RepositoryRulesetUpsert{
			Name:        testRulesetName,
			Target:      contractRulesetTargetBranch,
			Enforcement: contractRulesetActive,
			Rules:       []RulesetRule{{Type: contractRulesetDeletion}},
		})
		if _, ok := omitted[contractBypassActorsJSON]; ok {
			t.Fatalf("expected bypass_actors to be omitted, got %#v", omitted[contractBypassActorsJSON])
		}

		emptyActors := []RulesetBypassActor{}
		explicit := captureCreateRulesetPayload(t, RepositoryRulesetUpsert{
			Name:         testRulesetName,
			Target:       contractRulesetTargetBranch,
			Enforcement:  contractRulesetActive,
			BypassActors: &emptyActors,
			Rules:        []RulesetRule{{Type: contractRulesetDeletion}},
		})
		actors := requireJSONArrayContractTest(t, explicit, contractBypassActorsJSON)
		if len(actors) != 0 {
			t.Fatalf("expected empty bypass actor array, got %#v", actors)
		}
	})
}

func TestRepositoryRulesetValidationErrorContract(t *testing.T) {
	t.Parallel()

	const validationMessage = "Invalid property /conditions/ref_name/exclude: data cannot be null."
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", request.Method)
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = writer.Write([]byte(`{
			"message":"Invalid request.\\n\\n` + validationMessage + `",
			"documentation_url":"https://docs.github.com/rest/repos/rules#create-a-repository-ruleset",
			"status":"422"
		}`))
	}))
	defer server.Close()

	client, err := NewRESTClient(contractAccessToken, server.URL)
	if err != nil {
		t.Fatalf(contractCreateClientError, err)
	}
	_, err = client.CreateRepositoryRuleset(
		context.Background(),
		contractOrganization,
		contractRepository,
		RepositoryRulesetUpsert{
			Name:        testRulesetName,
			Target:      contractRulesetTargetBranch,
			Enforcement: contractRulesetActive,
			Rules:       []RulesetRule{{Type: contractRulesetDeletion}},
		},
	)
	if err == nil {
		t.Fatal("expected GitHub validation error")
	}
	if !strings.Contains(err.Error(), validationMessage) {
		t.Fatalf("expected validation detail in error, got %v", err)
	}
}

func TestListRepositoryRulesetsPaginationContract(t *testing.T) {
	t.Parallel()

	var pages []int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != testRulesetCollectionPath {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.String())
		}
		if request.URL.Query().Get("includes_parents") != "false" {
			t.Fatalf("parent rulesets must be excluded: %s", request.URL.RawQuery)
		}
		if request.URL.Query().Get("per_page") != "100" {
			t.Fatalf("unexpected page size: %s", request.URL.RawQuery)
		}
		page, err := strconv.Atoi(request.URL.Query().Get("page"))
		if err != nil {
			t.Fatalf("parse page: %v", err)
		}
		pages = append(pages, page)

		count := 100
		if page == 2 {
			count = 1
		}
		items := make([]map[string]any, 0, count)
		for i := 0; i < count; i++ {
			id := int64((page-1)*100 + i + 1)
			items = append(items, map[string]any{
				"id":          id,
				"name":        fmt.Sprintf("ruleset-%d", id),
				"source_type": "Repository",
				"source":      "k8sready/example",
				"enforcement": contractRulesetActive,
			})
		}
		if err := json.NewEncoder(writer).Encode(items); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewRESTClient(contractAccessToken, server.URL)
	if err != nil {
		t.Fatalf(contractCreateClientError, err)
	}
	items, err := client.ListRepositoryRulesets(context.Background(), contractOrganization, contractRepository)
	if err != nil {
		t.Fatalf("list repository rulesets: %v", err)
	}
	if len(items) != 101 {
		t.Fatalf("expected 101 rulesets, got %d", len(items))
	}
	if len(pages) != 2 || pages[0] != 1 || pages[1] != 2 {
		t.Fatalf("unexpected requested pages %#v", pages)
	}
}

func TestRepositoryRulesetNotFoundContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		call func(*RESTClient) error
	}{
		{
			name: "list",
			call: func(client *RESTClient) error {
				_, err := client.ListRepositoryRulesets(context.Background(), contractOrganization, contractRepository)
				return err
			},
		},
		{
			name: "get",
			call: func(client *RESTClient) error {
				_, err := client.GetRepositoryRuleset(context.Background(), contractOrganization, contractRepository, 42)
				return err
			},
		},
		{
			name: "delete",
			call: func(client *RESTClient) error {
				return client.DeleteRepositoryRuleset(context.Background(), contractOrganization, contractRepository, 42)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(http.StatusNotFound)
				_, _ = writer.Write([]byte(`{"message":"Not Found"}`))
			}))
			defer server.Close()

			client, err := NewRESTClient(contractAccessToken, server.URL)
			if err != nil {
				t.Fatalf(contractCreateClientError, err)
			}
			if err := test.call(client); !errors.Is(err, ErrNotFound) {
				t.Fatalf("expected ErrNotFound, got %v", err)
			}
		})
	}
}

func captureCreateRulesetPayload(
	t *testing.T,
	input RepositoryRulesetUpsert,
) map[string]any {
	t.Helper()

	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != testRulesetCollectionPath {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.String())
		}
		if request.Header.Get("X-GitHub-Api-Version") != githubAPIVersion {
			t.Fatalf("unexpected API version %q", request.Header.Get("X-GitHub-Api-Version"))
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request payload: %v", err)
		}
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(`{
			"id":42,
			"name":"` + testRulesetName + `",
			"target":"` + contractRulesetTargetBranch + `",
			"source_type":"Repository",
			"source":"k8sready/example",
			"enforcement":"` + contractRulesetActive + `",
			"rules":[]
		}`))
	}))
	defer server.Close()

	client, err := NewRESTClient(contractAccessToken, server.URL)
	if err != nil {
		t.Fatalf(contractCreateClientError, err)
	}
	if _, err := client.CreateRepositoryRuleset(
		context.Background(),
		contractOrganization,
		contractRepository,
		input,
	); err != nil {
		t.Fatalf("create ruleset: %v", err)
	}
	return payload
}

func requireJSONMapContractTest(t *testing.T, input map[string]any, key string) map[string]any {
	t.Helper()

	value, ok := input[key]
	if !ok {
		t.Fatalf("missing JSON property %q in %#v", key, input)
	}
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("JSON property %q must be an object, got %T (%#v)", key, value, value)
	}
	return result
}

func requireJSONArrayContractTest(t *testing.T, input map[string]any, key string) []any {
	t.Helper()

	value, ok := input[key]
	if !ok {
		t.Fatalf("missing JSON property %q in %#v", key, input)
	}
	result, ok := value.([]any)
	if !ok {
		t.Fatalf("JSON property %q must be an array, got %T (%#v)", key, value, value)
	}
	return result
}
