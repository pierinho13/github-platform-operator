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
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	githubv1alpha1 "github.com/pierinho13/github-platform-operator/api/v1alpha1"
	githubclient "github.com/pierinho13/github-platform-operator/internal/github"
)

const (
	githubRepositoryFinalizer = "github.k8sready.com/repository-finalizer"
	requeueInterval           = 5 * time.Minute
)

// GitHubRepositoryReconciler reconciles a GitHubRepository object.
type GitHubRepositoryReconciler struct {
	client.Client
	APIReader           client.Reader
	Scheme              *runtime.Scheme
	GitHubClientFactory githubclient.RepositoryClientFactory
}

// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubrepositories,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubrepositories/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubrepositories/finalizers,verbs=update
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubproviderconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get

// Reconcile moves the GitHub repository closer to the state declared in Kubernetes.
func (r *GitHubRepositoryReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	var repository githubv1alpha1.GitHubRepository
	if err := r.Get(ctx, req.NamespacedName, &repository); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !repository.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &repository)
	}

	if !controllerutil.ContainsFinalizer(&repository, githubRepositoryFinalizer) {
		controllerutil.AddFinalizer(&repository, githubRepositoryFinalizer)
		if err := r.Update(ctx, &repository); err != nil {
			return ctrl.Result{}, fmt.Errorf("add GitHubRepository finalizer: %w", err)
		}

		return ctrl.Result{}, nil
	}

	provider, repositoryClient, err := r.resolveProvider(ctx, &repository)
	if err != nil {
		statusErr := r.setReadyCondition(
			ctx,
			&repository,
			nil,
			nil,
			metav1.ConditionFalse,
			"ProviderUnavailable",
			err.Error(),
		)
		if statusErr != nil {
			return ctrl.Result{}, fmt.Errorf("%w; update failure status: %v", err, statusErr)
		}

		return ctrl.Result{}, err
	}

	desiredVisibility := "<unmanaged>"
	if repository.Spec.Visibility != nil {
		desiredVisibility = string(*repository.Spec.Visibility)
	}

	logger.Info(
		"reconciling GitHubRepository",
		"providerConfig", provider.Name,
		"organization", provider.Spec.Organization,
		"repository", repository.Spec.Name,
		"visibility", desiredVisibility,
		"deletionPolicy", repository.Spec.EffectiveDeletionPolicy(),
	)

	remoteRepository, action, err := r.reconcileRemoteRepository(
		ctx,
		&repository,
		provider.Spec.Organization,
		repositoryClient,
	)
	if err != nil {
		statusErr := r.setReadyCondition(
			ctx,
			&repository,
			provider,
			nil,
			metav1.ConditionFalse,
			"ReconciliationFailed",
			err.Error(),
		)
		if statusErr != nil {
			return ctrl.Result{}, fmt.Errorf("%w; update failure status: %v", err, statusErr)
		}

		return ctrl.Result{}, err
	}

	if err := r.setReadyCondition(
		ctx,
		&repository,
		provider,
		remoteRepository,
		metav1.ConditionTrue,
		action,
		fmt.Sprintf(
			"GitHub repository %s/%s is available with %s visibility",
			provider.Spec.Organization,
			repository.Spec.Name,
			remoteRepository.Visibility,
		),
	); err != nil {
		return ctrl.Result{}, fmt.Errorf("update GitHubRepository status: %w", err)
	}

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *GitHubRepositoryReconciler) resolveProvider(
	ctx context.Context,
	repository *githubv1alpha1.GitHubRepository,
) (*githubv1alpha1.GitHubProviderConfig, githubclient.RepositoryClient, error) {
	if r.GitHubClientFactory == nil {
		return nil, nil, errors.New("GitHub client factory is not configured")
	}

	providerName := repository.Spec.EffectiveProviderConfigRef()
	var provider githubv1alpha1.GitHubProviderConfig
	if err := r.Get(ctx, types.NamespacedName{Name: providerName}, &provider); err != nil {
		return nil, nil, fmt.Errorf("get GitHubProviderConfig %q: %w", providerName, err)
	}

	reader := r.APIReader
	if reader == nil {
		reader = r.Client
	}

	secretRef := provider.Spec.Credentials.SecretRef
	var secret corev1.Secret
	if err := reader.Get(ctx, types.NamespacedName{
		Namespace: secretRef.Namespace,
		Name:      secretRef.Name,
	}, &secret); err != nil {
		return nil, nil, fmt.Errorf(
			"get credentials Secret %s/%s: %w",
			secretRef.Namespace,
			secretRef.Name,
			err,
		)
	}

	tokenValue, ok := secret.Data[secretRef.Key]
	if !ok {
		return nil, nil, fmt.Errorf(
			"credentials Secret %s/%s does not contain key %q",
			secretRef.Namespace,
			secretRef.Name,
			secretRef.Key,
		)
	}

	token := strings.TrimSpace(string(tokenValue))
	if token == "" {
		return nil, nil, fmt.Errorf(
			"credentials Secret %s/%s contains an empty key %q",
			secretRef.Namespace,
			secretRef.Name,
			secretRef.Key,
		)
	}

	apiURL := provider.Spec.APIURL
	if apiURL == "" {
		apiURL = githubv1alpha1.DefaultGitHubAPIURL
	}

	repositoryClient, err := r.GitHubClientFactory.NewRepositoryClient(token, apiURL)
	if err != nil {
		return nil, nil, fmt.Errorf("create GitHub client for provider %q: %w", provider.Name, err)
	}

	return &provider, repositoryClient, nil
}

func (r *GitHubRepositoryReconciler) reconcileRemoteRepository(
	ctx context.Context,
	repository *githubv1alpha1.GitHubRepository,
	organization string,
	repositoryClient githubclient.RepositoryClient,
) (*githubclient.Repository, string, error) {
	logger := logf.FromContext(ctx)

	remoteRepository, err := repositoryClient.GetRepository(
		ctx,
		organization,
		repository.Spec.Name,
	)
	if err != nil && !errors.Is(err, githubclient.ErrNotFound) {
		return nil, "", fmt.Errorf(
			"get GitHub repository %s/%s: %w",
			organization,
			repository.Spec.Name,
			err,
		)
	}

	if errors.Is(err, githubclient.ErrNotFound) {
		logger.Info("GitHub repository does not exist, creating it")

		remoteRepository, err = repositoryClient.CreateRepository(
			ctx,
			organization,
			repository.Spec.Name,
			repository.Spec.EffectiveVisibilityForCreation() == githubv1alpha1.RepositoryVisibilityPrivate,
		)
		if err != nil {
			return nil, "", fmt.Errorf(
				"create GitHub repository %s/%s: %w",
				organization,
				repository.Spec.Name,
				err,
			)
		}

		logger.Info(
			"GitHub repository created",
			"repositoryID", remoteRepository.ID,
			"url", remoteRepository.HTMLURL,
		)

		return remoteRepository, "RepositoryCreated", nil
	}

	if repository.Spec.Visibility == nil {
		logger.Info(
			"preserving existing GitHub repository visibility",
			"visibility", remoteRepository.Visibility,
		)

		return remoteRepository, "RepositoryAvailable", nil
	}

	desiredVisibility := string(*repository.Spec.Visibility)
	if remoteRepository.Visibility != desiredVisibility {
		logger.Info(
			"updating GitHub repository visibility",
			"currentVisibility", remoteRepository.Visibility,
			"desiredVisibility", desiredVisibility,
		)

		remoteRepository, err = repositoryClient.UpdateRepositoryVisibility(
			ctx,
			organization,
			repository.Spec.Name,
			desiredVisibility,
		)
		if err != nil {
			return nil, "", fmt.Errorf(
				"update GitHub repository %s/%s visibility: %w",
				organization,
				repository.Spec.Name,
				err,
			)
		}

		logger.Info(
			"GitHub repository visibility updated",
			"visibility", remoteRepository.Visibility,
		)

		return remoteRepository, "RepositoryUpdated", nil
	}

	logger.Info(
		"GitHub repository already matches the desired state",
		"repositoryID", remoteRepository.ID,
		"url", remoteRepository.HTMLURL,
	)

	return remoteRepository, "RepositoryAvailable", nil
}

func (r *GitHubRepositoryReconciler) reconcileDelete(
	ctx context.Context,
	repository *githubv1alpha1.GitHubRepository,
) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(repository, githubRepositoryFinalizer) {
		return ctrl.Result{}, nil
	}

	if repository.Spec.EffectiveDeletionPolicy() == githubv1alpha1.RepositoryDeletionPolicyOrphan {
		logger.Info("orphaning GitHub repository and removing finalizer")

		controllerutil.RemoveFinalizer(repository, githubRepositoryFinalizer)
		if err := r.Update(ctx, repository); err != nil {
			return ctrl.Result{}, fmt.Errorf("remove GitHubRepository finalizer: %w", err)
		}

		return ctrl.Result{}, nil
	}

	provider, repositoryClient, err := r.resolveProvider(ctx, repository)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("resolve provider for deletion: %w", err)
	}

	logger.Info(
		"deleting GitHub repository before removing finalizer",
		"providerConfig", provider.Name,
		"organization", provider.Spec.Organization,
	)

	err = repositoryClient.DeleteRepository(
		ctx,
		provider.Spec.Organization,
		repository.Spec.Name,
	)
	if err != nil && !errors.Is(err, githubclient.ErrNotFound) {
		return ctrl.Result{}, fmt.Errorf(
			"delete GitHub repository %s/%s: %w",
			provider.Spec.Organization,
			repository.Spec.Name,
			err,
		)
	}

	if errors.Is(err, githubclient.ErrNotFound) {
		logger.Info("GitHub repository was already deleted")
	} else {
		logger.Info("GitHub repository deleted")
	}

	controllerutil.RemoveFinalizer(repository, githubRepositoryFinalizer)
	if err := r.Update(ctx, repository); err != nil {
		return ctrl.Result{}, fmt.Errorf("remove GitHubRepository finalizer: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *GitHubRepositoryReconciler) setReadyCondition(
	ctx context.Context,
	repository *githubv1alpha1.GitHubRepository,
	provider *githubv1alpha1.GitHubProviderConfig,
	remoteRepository *githubclient.Repository,
	conditionStatus metav1.ConditionStatus,
	reason string,
	message string,
) error {
	previousStatus := repository.Status.DeepCopy()
	repository.Status.ProviderConfigRef = repository.Spec.EffectiveProviderConfigRef()
	if provider != nil {
		repository.Status.Organization = provider.Spec.Organization
	}
	if remoteRepository != nil {
		repository.Status.RepositoryID = remoteRepository.ID
		repository.Status.URL = remoteRepository.HTMLURL
		repository.Status.Visibility = githubv1alpha1.RepositoryVisibility(remoteRepository.Visibility)
	}
	repository.Status.ObservedGeneration = repository.Generation

	meta.SetStatusCondition(&repository.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             conditionStatus,
		ObservedGeneration: repository.Generation,
		Reason:             reason,
		Message:            message,
	})

	if apiequality.Semantic.DeepEqual(previousStatus, &repository.Status) {
		return nil
	}

	return r.Status().Update(ctx, repository)
}

// SetupWithManager sets up the controller with the Manager.
func (r *GitHubRepositoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&githubv1alpha1.GitHubRepository{}).
		Named("githubrepository").
		Complete(r)
}
