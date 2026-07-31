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
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	githubv1alpha1 "github.com/pierinho13/github-platform-operator/api/v1alpha1"
	githubclient "github.com/pierinho13/github-platform-operator/internal/github"
)

const (
	organizationMembershipStatePending = "pending"
	invitationPendingReason            = "InvitationPending"
)

type resolvedOrganization struct {
	Provider *githubv1alpha1.GitHubProviderConfig
	Client   githubclient.OrganizationClient
}

func resolveOrganization(
	ctx context.Context,
	kubeClient client.Client,
	apiReader client.Reader,
	factory githubclient.OrganizationClientFactory,
	tokenProvider githubclient.TokenProvider,
	providerName string,
) (*resolvedOrganization, error) {
	if factory == nil {
		return nil, errors.New("GitHub organization client factory is not configured")
	}
	if providerName == "" {
		providerName = githubv1alpha1.DefaultGitHubProviderConfigName
	}

	var provider githubv1alpha1.GitHubProviderConfig
	if err := kubeClient.Get(ctx, types.NamespacedName{Name: providerName}, &provider); err != nil {
		return nil, fmt.Errorf("get GitHubProviderConfig %q: %w", providerName, err)
	}

	token, err := resolveProviderToken(
		ctx,
		kubeClient,
		apiReader,
		tokenProvider,
		&provider,
	)
	if err != nil {
		return nil, err
	}

	apiURL := provider.Spec.APIURL
	if apiURL == "" {
		apiURL = githubv1alpha1.DefaultGitHubAPIURL
	}
	organizationClient, err := factory.NewOrganizationClient(token, apiURL)
	if err != nil {
		return nil, fmt.Errorf("create GitHub organization client for provider %q: %w", provider.Name, err)
	}

	return &resolvedOrganization{Provider: &provider, Client: organizationClient}, nil
}

func findRemoteTeam(
	ctx context.Context,
	organizationClient githubclient.OrganizationClient,
	organization string,
	team *githubv1alpha1.GitHubTeam,
) (*githubclient.Team, error) {
	if team.Status.Slug != "" {
		remote, err := organizationClient.GetTeam(ctx, organization, team.Status.Slug)
		if err == nil {
			return remote, nil
		}
		if !errors.Is(err, githubclient.ErrNotFound) {
			return nil, err
		}
	}

	teams, err := organizationClient.ListTeams(ctx, organization)
	if err != nil {
		return nil, err
	}
	for i := range teams {
		if strings.EqualFold(strings.TrimSpace(teams[i].Name), strings.TrimSpace(team.Spec.Name)) {
			remote := teams[i]
			return &remote, nil
		}
	}
	return nil, githubclient.ErrNotFound
}
