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
	"time"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	Scheme       *runtime.Scheme
	GitHubClient githubclient.RepositoryClient
}

// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubrepositories,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubrepositories/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubrepositories/finalizers,verbs=update

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

	logger.Info(
		"reconciling GitHubRepository",
		"organization", repository.Spec.Organization,
		"repository", repository.Spec.Name,
		"visibility", repository.Spec.Visibility,
	)

	if r.GitHubClient == nil {
		return ctrl.Result{}, errors.New("GitHub client is not configured")
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

	remoteRepository, action, err := r.reconcileRemoteRepository(ctx, &repository)
	if err != nil {
		statusErr := r.setReadyCondition(
			ctx,
			&repository,
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
		remoteRepository,
		metav1.ConditionTrue,
		action,
		fmt.Sprintf(
			"GitHub repository %s/%s is available with %s visibility",
			repository.Spec.Organization,
			repository.Spec.Name,
			remoteRepository.Visibility,
		),
	); err != nil {
		return ctrl.Result{}, fmt.Errorf("update GitHubRepository status: %w", err)
	}

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *GitHubRepositoryReconciler) reconcileRemoteRepository(
	ctx context.Context,
	repository *githubv1alpha1.GitHubRepository,
) (*githubclient.Repository, string, error) {
	logger := logf.FromContext(ctx)

	remoteRepository, err := r.GitHubClient.GetRepository(
		ctx,
		repository.Spec.Organization,
		repository.Spec.Name,
	)
	if err != nil && !errors.Is(err, githubclient.ErrNotFound) {
		return nil, "", fmt.Errorf(
			"get GitHub repository %s/%s: %w",
			repository.Spec.Organization,
			repository.Spec.Name,
			err,
		)
	}

	if errors.Is(err, githubclient.ErrNotFound) {
		logger.Info("GitHub repository does not exist, creating it")

		remoteRepository, err = r.GitHubClient.CreateRepository(
			ctx,
			repository.Spec.Organization,
			repository.Spec.Name,
			repository.Spec.Visibility == githubv1alpha1.RepositoryVisibilityPrivate,
		)
		if err != nil {
			return nil, "", fmt.Errorf(
				"create GitHub repository %s/%s: %w",
				repository.Spec.Organization,
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

	desiredVisibility := string(repository.Spec.Visibility)
	if remoteRepository.Visibility != desiredVisibility {
		logger.Info(
			"updating GitHub repository visibility",
			"currentVisibility", remoteRepository.Visibility,
			"desiredVisibility", desiredVisibility,
		)

		remoteRepository, err = r.GitHubClient.UpdateRepositoryVisibility(
			ctx,
			repository.Spec.Organization,
			repository.Spec.Name,
			desiredVisibility,
		)
		if err != nil {
			return nil, "", fmt.Errorf(
				"update GitHub repository %s/%s visibility: %w",
				repository.Spec.Organization,
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

	logger.Info("deleting GitHub repository before removing finalizer")

	err := r.GitHubClient.DeleteRepository(
		ctx,
		repository.Spec.Organization,
		repository.Spec.Name,
	)
	if err != nil && !errors.Is(err, githubclient.ErrNotFound) {
		return ctrl.Result{}, fmt.Errorf(
			"delete GitHub repository %s/%s: %w",
			repository.Spec.Organization,
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
	remoteRepository *githubclient.Repository,
	conditionStatus metav1.ConditionStatus,
	reason string,
	message string,
) error {
	previousStatus := repository.Status.DeepCopy()

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
