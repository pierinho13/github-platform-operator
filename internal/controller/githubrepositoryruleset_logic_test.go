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

package controller

import (
	"encoding/json"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	githubv1alpha1 "github.com/pierinho13/github-platform-operator/api/v1alpha1"
	githubclient "github.com/pierinho13/github-platform-operator/internal/github"
)

const (
	contractRulesetName        = "protect-main"
	contractRulesetTarget      = "branch"
	contractRulesetActive      = "active"
	contractRulesetDeletion    = "deletion"
	contractRulesetPullRequest = "pull_request"
	contractDefaultBranch      = "~DEFAULT_BRANCH"
	contractBypassAlways       = "always"
	contractBuildDesiredError  = "build desired ruleset: %v"
)

func TestDesiredRepositoryRulesetContract(t *testing.T) {
	t.Parallel()

	t.Run("defaults target to branch and preserves empty ref collections", func(t *testing.T) {
		t.Parallel()

		desired, err := desiredRepositoryRuleset(githubv1alpha1.GitHubRepositoryRulesetSpec{
			Name:        contractRulesetName,
			Enforcement: githubv1alpha1.GitHubRulesetEnforcementActive,
			Conditions: &githubv1alpha1.GitHubRulesetConditions{
				RefName: &githubv1alpha1.GitHubRulesetRefNameCondition{
					Include: []string{contractDefaultBranch},
					Exclude: []string{},
				},
			},
			Rules: []githubv1alpha1.GitHubRulesetRule{{
				Type: githubv1alpha1.GitHubRulesetRuleDeletion,
			}},
		})
		if err != nil {
			t.Fatalf(contractBuildDesiredError, err)
		}
		if desired.Target != contractRulesetTarget {
			t.Fatalf("expected default branch target, got %q", desired.Target)
		}
		if desired.Conditions == nil || desired.Conditions.RefName == nil {
			t.Fatal("expected ref-name conditions")
		}
		if desired.Conditions.RefName.Include == nil {
			t.Fatal("expected include to be a non-nil collection")
		}
		if desired.Conditions.RefName.Exclude == nil {
			t.Fatal("expected exclude to be a non-nil collection")
		}
		if len(desired.Conditions.RefName.Exclude) != 0 {
			t.Fatalf("expected empty exclude, got %#v", desired.Conditions.RefName.Exclude)
		}
	})

	t.Run("turns an omitted exclude slice into an empty JSON collection", func(t *testing.T) {
		t.Parallel()

		desired, err := desiredRepositoryRuleset(githubv1alpha1.GitHubRepositoryRulesetSpec{
			Name:        contractRulesetName,
			Enforcement: githubv1alpha1.GitHubRulesetEnforcementActive,
			Conditions: &githubv1alpha1.GitHubRulesetConditions{
				RefName: &githubv1alpha1.GitHubRulesetRefNameCondition{
					Include: []string{contractDefaultBranch},
				},
			},
			Rules: []githubv1alpha1.GitHubRulesetRule{{
				Type: githubv1alpha1.GitHubRulesetRuleDeletion,
			}},
		})
		if err != nil {
			t.Fatalf(contractBuildDesiredError, err)
		}
		if desired.Conditions.RefName.Exclude == nil {
			t.Fatal("expected omitted exclude to become a non-nil empty collection")
		}
	})

	t.Run("keeps omitted conditions unmanaged", func(t *testing.T) {
		t.Parallel()

		desired, err := desiredRepositoryRuleset(minimalRulesetSpecForContractTest())
		if err != nil {
			t.Fatalf(contractBuildDesiredError, err)
		}
		if desired.Conditions != nil {
			t.Fatalf("expected conditions to remain unmanaged, got %#v", desired.Conditions)
		}
	})

	t.Run("distinguishes omitted and explicitly empty bypass actors", func(t *testing.T) {
		t.Parallel()

		omitted, err := desiredRepositoryRuleset(minimalRulesetSpecForContractTest())
		if err != nil {
			t.Fatalf("build ruleset with omitted bypass actors: %v", err)
		}
		if omitted.BypassActors != nil {
			t.Fatalf("expected omitted bypass actors to remain unmanaged, got %#v", omitted.BypassActors)
		}

		spec := minimalRulesetSpecForContractTest()
		spec.BypassActors = []githubv1alpha1.GitHubRulesetBypassActor{}
		explicit, err := desiredRepositoryRuleset(spec)
		if err != nil {
			t.Fatalf("build ruleset with empty bypass actors: %v", err)
		}
		if explicit.BypassActors == nil {
			t.Fatal("expected explicit empty bypass actors to be managed")
		}
		if len(*explicit.BypassActors) != 0 {
			t.Fatalf("expected no bypass actors, got %#v", *explicit.BypassActors)
		}
	})

	t.Run("defaults bypass mode and copies actor IDs", func(t *testing.T) {
		t.Parallel()

		actorID := int64(42)
		spec := minimalRulesetSpecForContractTest()
		spec.BypassActors = []githubv1alpha1.GitHubRulesetBypassActor{{
			ActorID:   &actorID,
			ActorType: githubv1alpha1.GitHubRulesetBypassActorTeam,
		}}

		desired, err := desiredRepositoryRuleset(spec)
		if err != nil {
			t.Fatalf(contractBuildDesiredError, err)
		}
		actor := (*desired.BypassActors)[0]
		if actor.BypassMode != contractBypassAlways {
			t.Fatalf("expected default bypass mode always, got %q", actor.BypassMode)
		}
		if actor.ActorID == nil || *actor.ActorID != 42 {
			t.Fatalf("unexpected actor ID %#v", actor.ActorID)
		}
		if actor.ActorID == &actorID {
			t.Fatal("expected actor ID to be copied")
		}
		actorID = 99
		if *actor.ActorID != 42 {
			t.Fatalf("desired actor ID changed through aliasing: %d", *actor.ActorID)
		}
	})

	t.Run("rejects invalid rule parameter JSON", func(t *testing.T) {
		t.Parallel()

		spec := minimalRulesetSpecForContractTest()
		spec.Rules = []githubv1alpha1.GitHubRulesetRule{{
			Type: githubv1alpha1.GitHubRulesetRulePullRequest,
			Parameters: &runtime.RawExtension{
				Raw: []byte(`{"broken":`),
			},
		}}
		if _, err := desiredRepositoryRuleset(spec); err == nil {
			t.Fatal("expected invalid JSON to be rejected")
		}
	})

	t.Run("copies raw rule parameters", func(t *testing.T) {
		t.Parallel()

		raw := []byte(`{"required_approving_review_count":1}`)
		spec := minimalRulesetSpecForContractTest()
		spec.Rules = []githubv1alpha1.GitHubRulesetRule{{
			Type:       githubv1alpha1.GitHubRulesetRulePullRequest,
			Parameters: &runtime.RawExtension{Raw: raw},
		}}

		desired, err := desiredRepositoryRuleset(spec)
		if err != nil {
			t.Fatalf(contractBuildDesiredError, err)
		}
		expected := string(desired.Rules[0].Parameters)
		raw[0] = 'X'
		if string(desired.Rules[0].Parameters) != expected {
			t.Fatalf("desired parameters were aliased: %s", desired.Rules[0].Parameters)
		}
	})
}

func TestRepositoryRulesetNeedsUpdateContract(t *testing.T) {
	t.Parallel()

	t.Run("is idempotent for equivalent desired and remote state", func(t *testing.T) {
		t.Parallel()
		desired := completeRulesetUpsertForContractTest()
		remote := repositoryRulesetFromUpsertForContractTest(desired)
		if repositoryRulesetNeedsUpdate(remote, desired) {
			t.Fatal("equivalent rulesets should not require an update")
		}
	})

	t.Run("treats JSON object key order as equivalent", func(t *testing.T) {
		t.Parallel()
		desired := completeRulesetUpsertForContractTest()
		remote := repositoryRulesetFromUpsertForContractTest(desired)
		remote.Rules[1].Parameters = json.RawMessage(`{"required_approving_review_count":1,"allowed_merge_methods":["squash"]}`)
		if repositoryRulesetNeedsUpdate(remote, desired) {
			t.Fatal("canonical JSON should prevent an unnecessary update")
		}
	})

	t.Run("treats rule order as equivalent", func(t *testing.T) {
		t.Parallel()
		desired := completeRulesetUpsertForContractTest()
		remote := repositoryRulesetFromUpsertForContractTest(desired)
		remote.Rules[0], remote.Rules[1] = remote.Rules[1], remote.Rules[0]
		if repositoryRulesetNeedsUpdate(remote, desired) {
			t.Fatal("rule ordering should not cause drift")
		}
	})

	t.Run("treats condition pattern order as equivalent", func(t *testing.T) {
		t.Parallel()
		desired := completeRulesetUpsertForContractTest()
		remote := repositoryRulesetFromUpsertForContractTest(desired)
		remote.Conditions.RefName.Include[0], remote.Conditions.RefName.Include[1] =
			remote.Conditions.RefName.Include[1], remote.Conditions.RefName.Include[0]
		remote.Conditions.RefName.Exclude[0], remote.Conditions.RefName.Exclude[1] =
			remote.Conditions.RefName.Exclude[1], remote.Conditions.RefName.Exclude[0]
		if repositoryRulesetNeedsUpdate(remote, desired) {
			t.Fatal("condition ordering should not cause drift")
		}
	})

	t.Run("treats bypass actor order and empty default mode as equivalent", func(t *testing.T) {
		t.Parallel()
		desired := completeRulesetUpsertForContractTest()
		remote := repositoryRulesetFromUpsertForContractTest(desired)
		remote.BypassActors[0], remote.BypassActors[1] = remote.BypassActors[1], remote.BypassActors[0]
		remote.BypassActors[1].BypassMode = ""
		if repositoryRulesetNeedsUpdate(remote, desired) {
			t.Fatal("equivalent bypass actors should not cause drift")
		}
	})

	t.Run("ignores unmanaged remote conditions", func(t *testing.T) {
		t.Parallel()
		desired := completeRulesetUpsertForContractTest()
		desired.Conditions = nil
		remote := repositoryRulesetFromUpsertForContractTest(completeRulesetUpsertForContractTest())
		remote.Conditions.RefName.Include = []string{"refs/heads/remote-only"}
		if repositoryRulesetNeedsUpdate(remote, desired) {
			t.Fatal("unmanaged conditions should be ignored")
		}
	})

	t.Run("ignores unmanaged remote bypass actors", func(t *testing.T) {
		t.Parallel()
		desired := completeRulesetUpsertForContractTest()
		desired.BypassActors = nil
		remote := repositoryRulesetFromUpsertForContractTest(completeRulesetUpsertForContractTest())
		remote.BypassActors = []githubclient.RulesetBypassActor{{ActorType: "OrganizationAdmin", BypassMode: contractBypassAlways}}
		if repositoryRulesetNeedsUpdate(remote, desired) {
			t.Fatal("unmanaged bypass actors should be ignored")
		}
	})

	t.Run("explicit empty bypass actors clear remote actors", func(t *testing.T) {
		t.Parallel()
		desired := completeRulesetUpsertForContractTest()
		empty := []githubclient.RulesetBypassActor{}
		desired.BypassActors = &empty
		remote := repositoryRulesetFromUpsertForContractTest(completeRulesetUpsertForContractTest())
		if !repositoryRulesetNeedsUpdate(remote, desired) {
			t.Fatal("explicit empty bypass actors should clear remote actors")
		}
	})

	t.Run("detects meaningful drift", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name   string
			mutate func(*githubclient.RepositoryRuleset)
		}{
			{
				name: "missing remote ruleset",
				mutate: func(remote *githubclient.RepositoryRuleset) {
					*remote = githubclient.RepositoryRuleset{}
				},
			},
			{
				name: "different name",
				mutate: func(remote *githubclient.RepositoryRuleset) {
					remote.Name = "different"
				},
			},
			{
				name: "different target",
				mutate: func(remote *githubclient.RepositoryRuleset) {
					remote.Target = "tag"
				},
			},
			{
				name: "different enforcement",
				mutate: func(remote *githubclient.RepositoryRuleset) {
					remote.Enforcement = "disabled"
				},
			},
			{
				name: "different condition",
				mutate: func(remote *githubclient.RepositoryRuleset) {
					remote.Conditions.RefName.Exclude = []string{"refs/heads/new"}
				},
			},
			{
				name: "different parameters",
				mutate: func(remote *githubclient.RepositoryRuleset) {
					remote.Rules[1].Parameters = json.RawMessage(`{"required_approving_review_count":2}`)
				},
			},
			{
				name: "missing rule",
				mutate: func(remote *githubclient.RepositoryRuleset) {
					remote.Rules = remote.Rules[:1]
				},
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()
				desired := completeRulesetUpsertForContractTest()
				remote := repositoryRulesetFromUpsertForContractTest(desired)
				test.mutate(remote)
				if !repositoryRulesetNeedsUpdate(remote, desired) {
					t.Fatal("expected drift to require an update")
				}
			})
		}
	})

	if !repositoryRulesetNeedsUpdate(nil, completeRulesetUpsertForContractTest()) {
		t.Fatal("a nil remote ruleset must require an update")
	}
}

func minimalRulesetSpecForContractTest() githubv1alpha1.GitHubRepositoryRulesetSpec {
	return githubv1alpha1.GitHubRepositoryRulesetSpec{
		Name:        contractRulesetName,
		Enforcement: githubv1alpha1.GitHubRulesetEnforcementActive,
		Rules: []githubv1alpha1.GitHubRulesetRule{{
			Type: githubv1alpha1.GitHubRulesetRuleDeletion,
		}},
	}
}

func completeRulesetUpsertForContractTest() githubclient.RepositoryRulesetUpsert {
	teamID := int64(7)
	userID := int64(11)
	actors := []githubclient.RulesetBypassActor{
		{ActorID: &teamID, ActorType: "Team", BypassMode: contractBypassAlways},
		{ActorID: &userID, ActorType: "User", BypassMode: contractRulesetPullRequest},
	}
	return githubclient.RepositoryRulesetUpsert{
		Name:         contractRulesetName,
		Target:       contractRulesetTarget,
		Enforcement:  contractRulesetActive,
		BypassActors: &actors,
		Conditions: &githubclient.RulesetConditions{
			RefName: &githubclient.RulesetRefNameCondition{
				Include: []string{contractDefaultBranch, "refs/heads/release/*"},
				Exclude: []string{"refs/heads/release/old", "refs/heads/tmp"},
			},
		},
		Rules: []githubclient.RulesetRule{
			{Type: contractRulesetDeletion},
			{
				Type:       contractRulesetPullRequest,
				Parameters: json.RawMessage(`{"allowed_merge_methods":["squash"],"required_approving_review_count":1}`),
			},
		},
	}
}

func repositoryRulesetFromUpsertForContractTest(
	input githubclient.RepositoryRulesetUpsert,
) *githubclient.RepositoryRuleset {
	result := &githubclient.RepositoryRuleset{
		ID:          42,
		Name:        input.Name,
		Target:      input.Target,
		SourceType:  repositoryRulesetSourceType,
		Source:      "k8sready/example",
		Enforcement: input.Enforcement,
		Rules:       make([]githubclient.RulesetRule, len(input.Rules)),
	}
	if input.BypassActors != nil {
		result.BypassActors = make([]githubclient.RulesetBypassActor, len(*input.BypassActors))
		for i := range *input.BypassActors {
			result.BypassActors[i] = (*input.BypassActors)[i]
			result.BypassActors[i].ActorID = copyInt64Pointer(result.BypassActors[i].ActorID)
		}
	}
	if input.Conditions != nil {
		result.Conditions = &githubclient.RulesetConditions{}
		if input.Conditions.RefName != nil {
			result.Conditions.RefName = &githubclient.RulesetRefNameCondition{
				Include: append([]string{}, input.Conditions.RefName.Include...),
				Exclude: append([]string{}, input.Conditions.RefName.Exclude...),
			}
		}
	}
	for i := range input.Rules {
		result.Rules[i] = input.Rules[i]
		result.Rules[i].Parameters = append(json.RawMessage(nil), input.Rules[i].Parameters...)
	}
	return result
}
