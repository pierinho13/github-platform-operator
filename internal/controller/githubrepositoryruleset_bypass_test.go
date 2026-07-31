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
	"context"
	"errors"
	"strings"
	"testing"

	githubv1alpha1 "github.com/pierinho13/github-platform-operator/api/v1alpha1"
	githubclient "github.com/pierinho13/github-platform-operator/internal/github"
)

const (
	bypassTestOrganization = "k8sready"
	bypassTestTeamSlug     = "platform"
	bypassTestUsername     = "octocat"
)

func TestResolveRepositoryRulesetBypassActors(t *testing.T) {
	t.Parallel()

	t.Run("resolves team slugs and usernames", func(t *testing.T) {
		t.Parallel()

		fakeClient := newFakeRepositoryRulesetClient()
		fakeClient.teamIDs[bypassTestOrganization+"/"+bypassTestTeamSlug] = 1001
		fakeClient.userIDs[bypassTestUsername] = 1002
		resolved := bypassTestResolvedRuleset(fakeClient)

		spec := githubv1alpha1.GitHubRepositoryRulesetSpec{
			BypassActors: []githubv1alpha1.GitHubRulesetBypassActor{
				{
					ActorType: githubv1alpha1.GitHubRulesetBypassActorTeam,
					TeamSlug:  bypassTestTeamSlug,
				},
				{
					ActorType: githubv1alpha1.GitHubRulesetBypassActorUser,
					Username:  bypassTestUsername,
				},
			},
		}

		result, err := resolveRepositoryRulesetBypassActors(context.Background(), spec, resolved)
		if err != nil {
			t.Fatalf("resolve bypass actors: %v", err)
		}
		if len(result.BypassActors) != 2 {
			t.Fatalf("expected two bypass actors, got %#v", result.BypassActors)
		}
		if result.BypassActors[0].ActorID == nil || *result.BypassActors[0].ActorID != 1001 {
			t.Fatalf("unexpected resolved team actor %#v", result.BypassActors[0])
		}
		if result.BypassActors[1].ActorID == nil || *result.BypassActors[1].ActorID != 1002 {
			t.Fatalf("unexpected resolved user actor %#v", result.BypassActors[1])
		}
		if fakeClient.teamLookupCalls != 1 || fakeClient.userLookupCalls != 1 {
			t.Fatalf(
				"unexpected lookup counts team=%d user=%d",
				fakeClient.teamLookupCalls,
				fakeClient.userLookupCalls,
			)
		}
	})

	t.Run("preserves explicit actor IDs without remote lookups", func(t *testing.T) {
		t.Parallel()

		fakeClient := newFakeRepositoryRulesetClient()
		resolved := bypassTestResolvedRuleset(fakeClient)
		teamID := int64(2001)
		userID := int64(2002)
		spec := githubv1alpha1.GitHubRepositoryRulesetSpec{
			BypassActors: []githubv1alpha1.GitHubRulesetBypassActor{
				{
					ActorType: githubv1alpha1.GitHubRulesetBypassActorTeam,
					ActorID:   &teamID,
				},
				{
					ActorType: githubv1alpha1.GitHubRulesetBypassActorUser,
					ActorID:   &userID,
				},
			},
		}

		result, err := resolveRepositoryRulesetBypassActors(context.Background(), spec, resolved)
		if err != nil {
			t.Fatalf("resolve bypass actors: %v", err)
		}
		if fakeClient.teamLookupCalls != 0 || fakeClient.userLookupCalls != 0 {
			t.Fatalf("explicit IDs must not trigger lookups")
		}
		if result.BypassActors[0].ActorID == &teamID || result.BypassActors[1].ActorID == &userID {
			t.Fatal("resolved actor IDs must be copied")
		}
	})

	t.Run("preserves omitted and explicitly empty management semantics", func(t *testing.T) {
		t.Parallel()

		resolved := bypassTestResolvedRuleset(newFakeRepositoryRulesetClient())
		omitted, err := resolveRepositoryRulesetBypassActors(
			context.Background(),
			githubv1alpha1.GitHubRepositoryRulesetSpec{},
			resolved,
		)
		if err != nil {
			t.Fatalf("resolve omitted actors: %v", err)
		}
		if omitted.BypassActors != nil {
			t.Fatalf("omitted bypass actors must remain nil, got %#v", omitted.BypassActors)
		}

		empty, err := resolveRepositoryRulesetBypassActors(
			context.Background(),
			githubv1alpha1.GitHubRepositoryRulesetSpec{
				BypassActors: []githubv1alpha1.GitHubRulesetBypassActor{},
			},
			resolved,
		)
		if err != nil {
			t.Fatalf("resolve empty actors: %v", err)
		}
		if empty.BypassActors == nil || len(empty.BypassActors) != 0 {
			t.Fatalf("explicit empty bypass actors must remain managed, got %#v", empty.BypassActors)
		}
	})

	t.Run("propagates team lookup failures", func(t *testing.T) {
		t.Parallel()

		_, err := resolveRepositoryRulesetBypassActors(
			context.Background(),
			githubv1alpha1.GitHubRepositoryRulesetSpec{
				BypassActors: []githubv1alpha1.GitHubRulesetBypassActor{{
					ActorType: githubv1alpha1.GitHubRulesetBypassActorTeam,
					TeamSlug:  bypassTestTeamSlug,
				}},
			},
			bypassTestResolvedRuleset(newFakeRepositoryRulesetClient()),
		)
		if !errors.Is(err, githubclient.ErrNotFound) {
			t.Fatalf("expected wrapped ErrNotFound, got %v", err)
		}
		if !strings.Contains(err.Error(), bypassTestTeamSlug) {
			t.Fatalf("expected team slug in error, got %v", err)
		}
	})

	t.Run("rejects ambiguous identifiers defensively", func(t *testing.T) {
		t.Parallel()

		actorID := int64(3001)
		_, err := resolveRepositoryRulesetBypassActors(
			context.Background(),
			githubv1alpha1.GitHubRepositoryRulesetSpec{
				BypassActors: []githubv1alpha1.GitHubRulesetBypassActor{{
					ActorType: githubv1alpha1.GitHubRulesetBypassActorTeam,
					ActorID:   &actorID,
					TeamSlug:  bypassTestTeamSlug,
				}},
			},
			bypassTestResolvedRuleset(newFakeRepositoryRulesetClient()),
		)
		if err == nil || !strings.Contains(err.Error(), "exactly one") {
			t.Fatalf("expected ambiguous identifier error, got %v", err)
		}
	})
}

func bypassTestResolvedRuleset(
	client githubclient.RepositoryRulesetClient,
) *resolvedRepositoryRuleset {
	return &resolvedRepositoryRuleset{
		Provider: &githubv1alpha1.GitHubProviderConfig{
			Spec: githubv1alpha1.GitHubProviderConfigSpec{
				Organization: bypassTestOrganization,
			},
		},
		Client: client,
	}
}
