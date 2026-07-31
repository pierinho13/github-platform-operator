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
	"strings"
	"time"

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
	GitHubTokenProvider githubclient.TokenProvider
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
			dependencyFailureReason(err),
			err.Error(),
		)
		if statusErr != nil {
			return ctrl.Result{}, fmt.Errorf("%w; update failure status: %v", err, statusErr)
		}

		if result, ok := githubDeferredResult(err); ok {
			return result, nil
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

		if result, ok := githubDeferredResult(err); ok {
			return result, nil
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

	token, err := resolveProviderToken(
		ctx,
		r.Client,
		r.APIReader,
		r.GitHubTokenProvider,
		&provider,
	)
	if err != nil {
		return nil, nil, err
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
	created := false
	updated := false

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

		createInput := githubclient.RepositoryCreate{
			Name:       repository.Spec.Name,
			Visibility: string(repository.Spec.EffectiveVisibilityForCreation()),
			AutoInit:   repository.Spec.EffectiveAutoInit(),
		}
		if repository.Spec.Template != nil {
			createInput.Template = &githubclient.RepositoryTemplateCreate{
				Owner:              repository.Spec.Template.Owner,
				Repository:         repository.Spec.Template.Repository,
				IncludeAllBranches: repository.Spec.Template.IncludeAllBranches,
			}
		}

		remoteRepository, err = repositoryClient.CreateRepository(
			ctx,
			organization,
			createInput,
		)
		if err != nil {
			return nil, "", fmt.Errorf(
				"create GitHub repository %s/%s: %w",
				organization,
				repository.Spec.Name,
				err,
			)
		}
		created = true

		logger.Info(
			"GitHub repository created",
			"repositoryID", remoteRepository.ID,
			"url", remoteRepository.HTMLURL,
		)
	}

	settingsUpdate := desiredRepositoryUpdate(repository.Spec, remoteRepository)
	if !settingsUpdate.Empty() {
		logger.Info("updating managed GitHub repository settings")

		remoteRepository, err = repositoryClient.UpdateRepository(
			ctx,
			organization,
			repository.Spec.Name,
			settingsUpdate,
		)
		if err != nil {
			return nil, "", fmt.Errorf(
				"update GitHub repository %s/%s settings: %w",
				organization,
				repository.Spec.Name,
				err,
			)
		}
		updated = true
	}

	if repository.Spec.Topics != nil {
		desiredTopics := normalizeRepositoryTopics(*repository.Spec.Topics)
		if !repositoryTopicsEqual(remoteRepository.Topics, desiredTopics) {
			logger.Info(
				"replacing managed GitHub repository topics",
				"topics", desiredTopics,
			)

			observedTopics, err := repositoryClient.ReplaceRepositoryTopics(
				ctx,
				organization,
				repository.Spec.Name,
				desiredTopics,
			)
			if err != nil {
				return nil, "", fmt.Errorf(
					"replace GitHub repository %s/%s topics: %w",
					organization,
					repository.Spec.Name,
					err,
				)
			}
			remoteRepository.Topics = observedTopics
			updated = true
		}
	}

	if repository.Spec.VulnerabilityAlerts != nil {
		observed, err := repositoryClient.GetRepositoryVulnerabilityAlerts(
			ctx,
			organization,
			repository.Spec.Name,
		)
		if err != nil {
			return nil, "", fmt.Errorf(
				"get GitHub repository %s/%s vulnerability alerts: %w",
				organization,
				repository.Spec.Name,
				err,
			)
		}
		if observed != *repository.Spec.VulnerabilityAlerts {
			logger.Info(
				"updating GitHub repository vulnerability alerts",
				"enabled", *repository.Spec.VulnerabilityAlerts,
			)
			if err := repositoryClient.SetRepositoryVulnerabilityAlerts(
				ctx,
				organization,
				repository.Spec.Name,
				*repository.Spec.VulnerabilityAlerts,
			); err != nil {
				return nil, "", fmt.Errorf(
					"update GitHub repository %s/%s vulnerability alerts: %w",
					organization,
					repository.Spec.Name,
					err,
				)
			}
			observed = *repository.Spec.VulnerabilityAlerts
			updated = true
		}
		remoteRepository.VulnerabilityAlerts = &observed
	}

	switch {
	case created:
		return remoteRepository, "RepositoryCreated", nil
	case updated:
		return remoteRepository, "RepositoryUpdated", nil
	default:
		logger.Info(
			"GitHub repository already matches the managed desired state",
			"repositoryID", remoteRepository.ID,
			"url", remoteRepository.HTMLURL,
		)
		return remoteRepository, "RepositoryAvailable", nil
	}
}

func desiredRepositoryUpdate(
	spec githubv1alpha1.GitHubRepositorySpec,
	remote *githubclient.Repository,
) githubclient.RepositoryUpdate {
	var update githubclient.RepositoryUpdate

	applyRepositoryCoreUpdate(&update, spec, remote)
	applyRepositoryFeaturesUpdate(&update, spec.Features, remote)
	applyRepositoryMergeOptionsUpdate(&update, spec.MergeOptions, remote)

	return update
}

func applyRepositoryCoreUpdate(
	update *githubclient.RepositoryUpdate,
	spec githubv1alpha1.GitHubRepositorySpec,
	remote *githubclient.Repository,
) {
	if spec.Visibility != nil && remote.Visibility != string(*spec.Visibility) {
		visibility := string(*spec.Visibility)
		update.Visibility = &visibility
	}
	if spec.Description != nil && remote.Description != *spec.Description {
		update.Description = spec.Description
	}
	if spec.Homepage != nil && remote.Homepage != *spec.Homepage {
		update.Homepage = spec.Homepage
	}
	if spec.DeleteBranchOnMerge != nil &&
		remote.DeleteBranchOnMerge != *spec.DeleteBranchOnMerge {
		update.DeleteBranchOnMerge = spec.DeleteBranchOnMerge
	}
	if spec.IsTemplate != nil && remote.IsTemplate != *spec.IsTemplate {
		update.IsTemplate = spec.IsTemplate
	}
}

func applyRepositoryFeaturesUpdate(
	update *githubclient.RepositoryUpdate,
	features *githubv1alpha1.RepositoryFeatures,
	remote *githubclient.Repository,
) {
	if features == nil {
		return
	}
	if features.Issues != nil && remote.HasIssues != *features.Issues {
		update.HasIssues = features.Issues
	}
	if features.Projects != nil && remote.HasProjects != *features.Projects {
		update.HasProjects = features.Projects
	}
	if features.Wiki != nil && remote.HasWiki != *features.Wiki {
		update.HasWiki = features.Wiki
	}
	if features.Discussions != nil && remote.HasDiscussions != *features.Discussions {
		update.HasDiscussions = features.Discussions
	}
}

func applyRepositoryMergeOptionsUpdate(
	update *githubclient.RepositoryUpdate,
	merge *githubv1alpha1.RepositoryMergeOptions,
	remote *githubclient.Repository,
) {
	if merge == nil {
		return
	}
	if merge.AllowAutoMerge != nil && remote.AllowAutoMerge != *merge.AllowAutoMerge {
		update.AllowAutoMerge = merge.AllowAutoMerge
	}
	if merge.AllowMergeCommit != nil && remote.AllowMergeCommit != *merge.AllowMergeCommit {
		update.AllowMergeCommit = merge.AllowMergeCommit
	}
	if merge.AllowRebaseMerge != nil && remote.AllowRebaseMerge != *merge.AllowRebaseMerge {
		update.AllowRebaseMerge = merge.AllowRebaseMerge
	}
	if merge.AllowSquashMerge != nil && remote.AllowSquashMerge != *merge.AllowSquashMerge {
		update.AllowSquashMerge = merge.AllowSquashMerge
	}
	if merge.MergeCommitTitle != nil &&
		remote.MergeCommitTitle != string(*merge.MergeCommitTitle) {
		value := string(*merge.MergeCommitTitle)
		update.MergeCommitTitle = &value
	}
	if merge.MergeCommitMessage != nil &&
		remote.MergeCommitMessage != string(*merge.MergeCommitMessage) {
		value := string(*merge.MergeCommitMessage)
		update.MergeCommitMessage = &value
	}
	if merge.SquashMergeCommitTitle != nil &&
		remote.SquashMergeCommitTitle != string(*merge.SquashMergeCommitTitle) {
		value := string(*merge.SquashMergeCommitTitle)
		update.SquashMergeCommitTitle = &value
	}
	if merge.SquashMergeCommitMessage != nil &&
		remote.SquashMergeCommitMessage != string(*merge.SquashMergeCommitMessage) {
		value := string(*merge.SquashMergeCommitMessage)
		update.SquashMergeCommitMessage = &value
	}
}

func normalizeRepositoryTopics(topics []string) []string {
	normalized := make([]string, 0, len(topics))
	seen := make(map[string]struct{}, len(topics))
	for _, topic := range topics {
		topic = strings.ToLower(strings.TrimSpace(topic))
		if topic == "" {
			continue
		}
		if _, ok := seen[topic]; ok {
			continue
		}
		seen[topic] = struct{}{}
		normalized = append(normalized, topic)
	}
	sort.Strings(normalized)
	return normalized
}

func repositoryTopicsEqual(current, desired []string) bool {
	current = normalizeRepositoryTopics(current)
	desired = normalizeRepositoryTopics(desired)
	if len(current) != len(desired) {
		return false
	}
	for i := range current {
		if current[i] != desired[i] {
			return false
		}
	}
	return true
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
		if result, ok := githubDeferredResult(err); ok {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolve provider for deletion: %w", err)
	}

	if repository.Spec.EffectiveDeletionPolicy() == githubv1alpha1.RepositoryDeletionPolicyArchive {
		logger.Info(
			"archiving GitHub repository before removing finalizer",
			"providerConfig", provider.Name,
			"organization", provider.Spec.Organization,
		)
		_, err = repositoryClient.ArchiveRepository(
			ctx,
			provider.Spec.Organization,
			repository.Spec.Name,
		)
		if err != nil && !errors.Is(err, githubclient.ErrNotFound) {
			if result, ok := githubDeferredResult(err); ok {
				return result, nil
			}
			return ctrl.Result{}, fmt.Errorf(
				"archive GitHub repository %s/%s: %w",
				provider.Spec.Organization,
				repository.Spec.Name,
				err,
			)
		}
		controllerutil.RemoveFinalizer(repository, githubRepositoryFinalizer)
		if err := r.Update(ctx, repository); err != nil {
			return ctrl.Result{}, fmt.Errorf("remove GitHubRepository finalizer: %w", err)
		}
		return ctrl.Result{}, nil
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
		if result, ok := githubDeferredResult(err); ok {
			return result, nil
		}
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
		repository.Status.Description = remoteRepository.Description
		repository.Status.Homepage = remoteRepository.Homepage
		repository.Status.Topics = append([]string(nil), remoteRepository.Topics...)
		repository.Status.Features = &githubv1alpha1.RepositoryFeaturesStatus{
			Issues:      remoteRepository.HasIssues,
			Projects:    remoteRepository.HasProjects,
			Wiki:        remoteRepository.HasWiki,
			Discussions: remoteRepository.HasDiscussions,
		}
		repository.Status.DeleteBranchOnMerge = remoteRepository.DeleteBranchOnMerge
		repository.Status.VulnerabilityAlerts = copyBoolPointer(remoteRepository.VulnerabilityAlerts)
		repository.Status.IsTemplate = remoteRepository.IsTemplate
		repository.Status.Archived = remoteRepository.Archived
		repository.Status.MergeOptions = &githubv1alpha1.RepositoryMergeOptionsStatus{
			AllowAutoMerge:           remoteRepository.AllowAutoMerge,
			AllowMergeCommit:         remoteRepository.AllowMergeCommit,
			AllowRebaseMerge:         remoteRepository.AllowRebaseMerge,
			AllowSquashMerge:         remoteRepository.AllowSquashMerge,
			MergeCommitTitle:         githubv1alpha1.RepositoryMergeCommitTitle(remoteRepository.MergeCommitTitle),
			MergeCommitMessage:       githubv1alpha1.RepositoryMergeCommitMessage(remoteRepository.MergeCommitMessage),
			SquashMergeCommitTitle:   githubv1alpha1.RepositorySquashMergeCommitTitle(remoteRepository.SquashMergeCommitTitle),
			SquashMergeCommitMessage: githubv1alpha1.RepositorySquashMergeCommitMessage(remoteRepository.SquashMergeCommitMessage),
		}
	}
	repository.Status.ObservedGeneration = repository.Generation

	meta.SetStatusCondition(&repository.Status.Conditions, metav1.Condition{
		Type:               conditionTypeReady,
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

func copyBoolPointer(value *bool) *bool {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

// SetupWithManager sets up the controller with the Manager.
func (r *GitHubRepositoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&githubv1alpha1.GitHubRepository{}).
		Named("githubrepository").
		Complete(r)
}
