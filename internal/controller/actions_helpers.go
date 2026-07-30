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
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	githubv1alpha1 "github.com/pierinho13/github-platform-operator/api/v1alpha1"
	githubclient "github.com/pierinho13/github-platform-operator/internal/github"
)

type resolvedActionsTarget struct {
	Target                githubclient.ActionsTarget
	Provider              *githubv1alpha1.GitHubProviderConfig
	Repository            *githubv1alpha1.GitHubRepository
	Environment           *githubv1alpha1.GitHubEnvironment
	Visibility            githubv1alpha1.OrganizationActionsVisibility
	SelectedRepositoryIDs []int64
	Client                githubclient.ActionsClient
}

type resolvedValueSource struct {
	Value           []byte
	SecretUID       string
	ResourceVersion string
}

func resolveActionsTarget(
	ctx context.Context,
	kubeClient client.Client,
	apiReader client.Reader,
	factory githubclient.ActionsClientFactory,
	tokenProvider githubclient.TokenProvider,
	namespace string,
	target githubv1alpha1.GitHubActionsTarget,
) (*resolvedActionsTarget, error) {
	return resolveActionsTargetWithValidation(
		ctx, kubeClient, apiReader, factory, tokenProvider, namespace, target, true,
	)
}

func resolveActionsTargetForDeletion(
	ctx context.Context,
	kubeClient client.Client,
	apiReader client.Reader,
	factory githubclient.ActionsClientFactory,
	tokenProvider githubclient.TokenProvider,
	namespace string,
	target githubv1alpha1.GitHubActionsTarget,
) (*resolvedActionsTarget, error) {
	return resolveActionsTargetWithValidation(
		ctx, kubeClient, apiReader, factory, tokenProvider, namespace, target, false,
	)
}

func resolveActionsTargetWithValidation(
	ctx context.Context,
	kubeClient client.Client,
	apiReader client.Reader,
	factory githubclient.ActionsClientFactory,
	tokenProvider githubclient.TokenProvider,
	namespace string,
	target githubv1alpha1.GitHubActionsTarget,
	verifyRemote bool,
) (*resolvedActionsTarget, error) {
	if factory == nil {
		return nil, errors.New("GitHub Actions client factory is not configured")
	}

	switch target.Scope() {
	case githubv1alpha1.ActionsTargetScopeRepository:
		return resolveRepositoryActionsTarget(
			ctx, kubeClient, apiReader, factory, tokenProvider, namespace, *target.RepositoryRef, verifyRemote,
		)
	case githubv1alpha1.ActionsTargetScopeEnvironment:
		var environment githubv1alpha1.GitHubEnvironment
		if err := kubeClient.Get(ctx, types.NamespacedName{
			Namespace: namespace,
			Name:      target.EnvironmentRef.Name,
		}, &environment); err != nil {
			return nil, fmt.Errorf(
				"get GitHubEnvironment %s/%s: %w",
				namespace,
				target.EnvironmentRef.Name,
				err,
			)
		}

		resolved, err := resolveRepositoryActionsTarget(
			ctx,
			kubeClient,
			apiReader,
			factory,
			tokenProvider,
			namespace,
			environment.Spec.RepositoryRef,
			verifyRemote,
		)
		if err != nil {
			return nil, err
		}
		resolved.Environment = &environment
		resolved.Target.Scope = githubclient.ActionsTargetScopeEnvironment
		resolved.Target.Environment = environment.Spec.Name

		if verifyRemote {
			if _, err := resolved.Client.GetEnvironment(
				ctx,
				resolved.Provider.Spec.Organization,
				resolved.Repository.Spec.Name,
				environment.Spec.Name,
			); err != nil {
				if errors.Is(err, githubclient.ErrNotFound) {
					return nil, fmt.Errorf(
						"GitHub environment %s/%s/%s does not exist",
						resolved.Provider.Spec.Organization,
						resolved.Repository.Spec.Name,
						environment.Spec.Name,
					)
				}
				return nil, fmt.Errorf("get GitHub environment %q: %w", environment.Spec.Name, err)
			}
		}
		return resolved, nil
	case githubv1alpha1.ActionsTargetScopeOrganization:
		organizationTarget := target.Organization
		providerName := organizationTarget.EffectiveProviderConfigRef()
		provider, actionsClient, err := resolveActionsProvider(
			ctx, kubeClient, apiReader, factory, tokenProvider, providerName,
		)
		if err != nil {
			return nil, err
		}

		visibility := organizationTarget.EffectiveVisibility()
		if visibility == githubv1alpha1.OrganizationActionsVisibilitySelected &&
			len(organizationTarget.SelectedRepositoryRefs) == 0 {
			return nil, errors.New("selected organization visibility requires selectedRepositoryRefs")
		}
		if visibility != githubv1alpha1.OrganizationActionsVisibilitySelected &&
			len(organizationTarget.SelectedRepositoryRefs) != 0 {
			return nil, errors.New("selectedRepositoryRefs can only be used with selected visibility")
		}

		selectedIDs := make([]int64, 0, len(organizationTarget.SelectedRepositoryRefs))
		if verifyRemote {
			for i := range organizationTarget.SelectedRepositoryRefs {
				ref := organizationTarget.SelectedRepositoryRefs[i]
				var repository githubv1alpha1.GitHubRepository
				if err := kubeClient.Get(ctx, types.NamespacedName{
					Namespace: namespace,
					Name:      ref.Name,
				}, &repository); err != nil {
					return nil, fmt.Errorf("get selected GitHubRepository %s/%s: %w", namespace, ref.Name, err)
				}
				if repository.Spec.EffectiveProviderConfigRef() != provider.Name {
					return nil, fmt.Errorf(
						"selected GitHubRepository %s/%s uses provider %q instead of %q",
						namespace,
						repository.Name,
						repository.Spec.EffectiveProviderConfigRef(),
						provider.Name,
					)
				}
				remote, err := actionsClient.GetRepository(
					ctx,
					provider.Spec.Organization,
					repository.Spec.Name,
				)
				if err != nil {
					return nil, fmt.Errorf(
						"get selected GitHub repository %s/%s: %w",
						provider.Spec.Organization,
						repository.Spec.Name,
						err,
					)
				}
				selectedIDs = append(selectedIDs, remote.ID)
			}
			sort.Slice(selectedIDs, func(i, j int) bool { return selectedIDs[i] < selectedIDs[j] })
		}

		return &resolvedActionsTarget{
			Target: githubclient.ActionsTarget{
				Scope:        githubclient.ActionsTargetScopeOrganization,
				Organization: provider.Spec.Organization,
			},
			Provider:              provider,
			Visibility:            visibility,
			SelectedRepositoryIDs: selectedIDs,
			Client:                actionsClient,
		}, nil
	default:
		return nil, errors.New("exactly one GitHub Actions target must be configured")
	}
}

func resolveRepositoryActionsTarget(
	ctx context.Context,
	kubeClient client.Client,
	apiReader client.Reader,
	factory githubclient.ActionsClientFactory,
	tokenProvider githubclient.TokenProvider,
	namespace string,
	repositoryRef githubv1alpha1.GitHubRepositoryReference,
	verifyRemote bool,
) (*resolvedActionsTarget, error) {
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

	provider, actionsClient, err := resolveActionsProvider(
		ctx,
		kubeClient,
		apiReader,
		factory,
		tokenProvider,
		repository.Spec.EffectiveProviderConfigRef(),
	)
	if err != nil {
		return nil, err
	}

	if verifyRemote {
		if _, err := actionsClient.GetRepository(
			ctx,
			provider.Spec.Organization,
			repository.Spec.Name,
		); err != nil {
			if errors.Is(err, githubclient.ErrNotFound) {
				return nil, fmt.Errorf(
					"GitHub repository %s/%s does not exist",
					provider.Spec.Organization,
					repository.Spec.Name,
				)
			}
			return nil, fmt.Errorf(
				"get GitHub repository %s/%s: %w",
				provider.Spec.Organization,
				repository.Spec.Name,
				err,
			)
		}
	}

	return &resolvedActionsTarget{
		Target: githubclient.ActionsTarget{
			Scope:        githubclient.ActionsTargetScopeRepository,
			Organization: provider.Spec.Organization,
			Repository:   repository.Spec.Name,
		},
		Provider:   provider,
		Repository: &repository,
		Client:     actionsClient,
	}, nil
}

func resolveActionsProvider(
	ctx context.Context,
	kubeClient client.Client,
	apiReader client.Reader,
	factory githubclient.ActionsClientFactory,
	tokenProvider githubclient.TokenProvider,
	providerName string,
) (*githubv1alpha1.GitHubProviderConfig, githubclient.ActionsClient, error) {
	var provider githubv1alpha1.GitHubProviderConfig
	if err := kubeClient.Get(ctx, types.NamespacedName{Name: providerName}, &provider); err != nil {
		return nil, nil, fmt.Errorf("get GitHubProviderConfig %q: %w", providerName, err)
	}

	token, err := resolveProviderToken(
		ctx,
		kubeClient,
		apiReader,
		tokenProvider,
		&provider,
	)
	if err != nil {
		return nil, nil, err
	}

	apiURL := provider.Spec.APIURL
	if apiURL == "" {
		apiURL = githubv1alpha1.DefaultGitHubAPIURL
	}
	actionsClient, err := factory.NewActionsClient(token, apiURL)
	if err != nil {
		return nil, nil, fmt.Errorf("create GitHub Actions client for provider %q: %w", provider.Name, err)
	}
	return &provider, actionsClient, nil
}

func resolveActionsValueSource(
	ctx context.Context,
	reader client.Reader,
	namespace string,
	source githubv1alpha1.ActionsValueSource,
) (*resolvedValueSource, error) {
	var secret corev1.Secret
	if err := reader.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      source.SecretKeyRef.Name,
	}, &secret); err != nil {
		return nil, fmt.Errorf(
			"get source Secret %s/%s: %w",
			namespace,
			source.SecretKeyRef.Name,
			err,
		)
	}
	value, ok := secret.Data[source.SecretKeyRef.Key]
	if !ok {
		return nil, fmt.Errorf(
			"source Secret %s/%s does not contain key %q",
			namespace,
			source.SecretKeyRef.Name,
			source.SecretKeyRef.Key,
		)
	}
	return &resolvedValueSource{
		Value:           append([]byte(nil), value...),
		SecretUID:       string(secret.UID),
		ResourceVersion: secret.ResourceVersion,
	}, nil
}

func equalInt64Sets(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	aCopy := append([]int64(nil), a...)
	bCopy := append([]int64(nil), b...)
	sort.Slice(aCopy, func(i, j int) bool { return aCopy[i] < aCopy[j] })
	sort.Slice(bCopy, func(i, j int) bool { return bCopy[i] < bCopy[j] })
	for i := range aCopy {
		if aCopy[i] != bCopy[i] {
			return false
		}
	}
	return true
}
