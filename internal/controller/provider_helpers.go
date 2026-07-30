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
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	githubv1alpha1 "github.com/pierinho13/github-platform-operator/api/v1alpha1"
	githubclient "github.com/pierinho13/github-platform-operator/internal/github"
)

var (
	errProviderSuspended        = errors.New("GitHubProviderConfig is suspended")
	fallbackGitHubTokenProvider = githubclient.NewCachedTokenProvider(nil)
)

func resolveProviderToken(
	ctx context.Context,
	kubeClient client.Client,
	apiReader client.Reader,
	tokenProvider githubclient.TokenProvider,
	provider *githubv1alpha1.GitHubProviderConfig,
) (string, error) {
	if provider.Spec.Suspended {
		return "", fmt.Errorf("%w: %s", errProviderSuspended, provider.Name)
	}

	reader := apiReader
	if reader == nil {
		reader = kubeClient
	}

	apiURL := provider.Spec.APIURL
	if apiURL == "" {
		apiURL = githubv1alpha1.DefaultGitHubAPIURL
	}

	credentials := provider.Spec.Credentials
	switch {
	case credentials.SecretRef != nil && credentials.GitHubApp == nil:
		token, err := readSecretValue(ctx, reader, *credentials.SecretRef)
		if err != nil {
			return "", err
		}
		return token, nil
	case credentials.SecretRef == nil && credentials.GitHubApp != nil:
		privateKey, err := readSecretValueBytes(
			ctx,
			reader,
			credentials.GitHubApp.PrivateKeySecretRef,
		)
		if err != nil {
			return "", err
		}

		if tokenProvider == nil {
			tokenProvider = fallbackGitHubTokenProvider
		}
		token, err := tokenProvider.ResolveToken(
			ctx,
			githubclient.Authentication{
				GitHubApp: &githubclient.GitHubAppAuthentication{
					AppID:          credentials.GitHubApp.AppID,
					InstallationID: credentials.GitHubApp.InstallationID,
					PrivateKeyPEM:  privateKey,
				},
			},
			apiURL,
		)
		if err != nil {
			return "", fmt.Errorf(
				"resolve GitHub App installation token for provider %q: %w",
				provider.Name,
				err,
			)
		}
		return token, nil
	default:
		return "", fmt.Errorf(
			"provider %q must configure exactly one of credentials.secretRef or credentials.githubApp",
			provider.Name,
		)
	}
}

func readSecretValue(
	ctx context.Context,
	reader client.Reader,
	ref githubv1alpha1.NamespacedSecretKeyReference,
) (string, error) {
	value, err := readSecretValueBytes(ctx, reader, ref)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(value))
	if token == "" {
		return "", fmt.Errorf(
			"credentials Secret %s/%s contains an empty key %q",
			ref.Namespace,
			ref.Name,
			ref.Key,
		)
	}
	return token, nil
}

func readSecretValueBytes(
	ctx context.Context,
	reader client.Reader,
	ref githubv1alpha1.NamespacedSecretKeyReference,
) ([]byte, error) {
	var secret corev1.Secret
	if err := reader.Get(ctx, types.NamespacedName{
		Namespace: ref.Namespace,
		Name:      ref.Name,
	}, &secret); err != nil {
		return nil, fmt.Errorf(
			"get credentials Secret %s/%s: %w",
			ref.Namespace,
			ref.Name,
			err,
		)
	}

	value, ok := secret.Data[ref.Key]
	if !ok {
		return nil, fmt.Errorf(
			"credentials Secret %s/%s does not contain key %q",
			ref.Namespace,
			ref.Name,
			ref.Key,
		)
	}
	if len(strings.TrimSpace(string(value))) == 0 {
		return nil, fmt.Errorf(
			"credentials Secret %s/%s contains an empty key %q",
			ref.Namespace,
			ref.Name,
			ref.Key,
		)
	}

	return append([]byte(nil), value...), nil
}

func dependencyFailureReason(err error) string {
	if errors.Is(err, errProviderSuspended) {
		return "ReconciliationSuspended"
	}
	return "DependencyUnavailable"
}

func githubRateLimitResult(err error) (ctrl.Result, bool) {
	var rateLimitErr *githubclient.RateLimitError
	if !errors.As(err, &rateLimitErr) {
		return ctrl.Result{}, false
	}
	return ctrl.Result{RequeueAfter: rateLimitErr.RetryAfter()}, true
}

func githubDeferredResult(err error) (ctrl.Result, bool) {
	if result, ok := githubRateLimitResult(err); ok {
		return result, true
	}
	if errors.Is(err, errProviderSuspended) {
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, true
	}
	return ctrl.Result{}, false
}
