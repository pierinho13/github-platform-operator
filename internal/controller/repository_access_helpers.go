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
	"net/http"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	githubv1alpha1 "github.com/pierinho13/github-platform-operator/api/v1alpha1"
	githubclient "github.com/pierinho13/github-platform-operator/internal/github"
)

type resolvedRepositoryAccess struct {
	Repository *githubv1alpha1.GitHubRepository
	Provider   *githubv1alpha1.GitHubProviderConfig
	Client     githubclient.RepositoryAccessClient
}

func resolveRepositoryAccess(
	ctx context.Context,
	kubeClient client.Client,
	apiReader client.Reader,
	factory githubclient.RepositoryAccessClientFactory,
	tokenProvider githubclient.TokenProvider,
	namespace string,
	repositoryRef githubv1alpha1.GitHubRepositoryReference,
) (*resolvedRepositoryAccess, error) {
	if factory == nil {
		return nil, errors.New("GitHub repository access client factory is not configured")
	}

	var repository githubv1alpha1.GitHubRepository
	if err := kubeClient.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      repositoryRef.Name,
	}, &repository); err != nil {
		return nil, fmt.Errorf(
			"get GitHubRepository %s/%s: %w",
			namespace,
			repositoryRef.Name,
			err,
		)
	}

	providerName := repository.Spec.EffectiveProviderConfigRef()
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

	accessClient, err := factory.NewRepositoryAccessClient(token, apiURL)
	if err != nil {
		return nil, fmt.Errorf("create GitHub access client for provider %q: %w", provider.Name, err)
	}

	return &resolvedRepositoryAccess{
		Repository: &repository,
		Provider:   &provider,
		Client:     accessClient,
	}, nil
}

func getRemoteRepository(
	ctx context.Context,
	resolved *resolvedRepositoryAccess,
) (*githubclient.Repository, error) {
	repository, err := resolved.Client.GetRepository(
		ctx,
		resolved.Provider.Spec.Organization,
		resolved.Repository.Spec.Name,
	)
	if err != nil {
		if errors.Is(err, githubclient.ErrNotFound) {
			return nil, fmt.Errorf(
				"GitHub repository %s/%s does not exist",
				resolved.Provider.Spec.Organization,
				resolved.Repository.Spec.Name,
			)
		}

		return nil, fmt.Errorf(
			"get GitHub repository %s/%s: %w",
			resolved.Provider.Spec.Organization,
			resolved.Repository.Spec.Name,
			err,
		)
	}

	return repository, nil
}

func verifyRemoteRepository(
	ctx context.Context,
	resolved *resolvedRepositoryAccess,
) error {
	_, err := getRemoteRepository(ctx, resolved)
	return err
}

func isArchivedRepositoryError(err error) bool {
	var apiErr *githubclient.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden {
		return false
	}

	body := strings.ToLower(apiErr.Body)
	return strings.Contains(body, "repository was archived") ||
		strings.Contains(body, "repository is archived")
}
