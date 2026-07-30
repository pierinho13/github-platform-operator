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
	githubRepositoryTeamAccessFinalizer = "github.k8sready.com/team-access-finalizer"
	repositoryAccessRequeueInterval     = 5 * time.Minute
)

// GitHubRepositoryTeamAccessReconciler reconciles GitHubRepositoryTeamAccess resources.
type GitHubRepositoryTeamAccessReconciler struct {
	client.Client
	APIReader           client.Reader
	Scheme              *runtime.Scheme
	GitHubClientFactory githubclient.RepositoryAccessClientFactory
}

// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubrepositoryteamaccesses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubrepositoryteamaccesses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubrepositoryteamaccesses/finalizers,verbs=update
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubrepositories;githubproviderconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get

func (r *GitHubRepositoryTeamAccessReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	var access githubv1alpha1.GitHubRepositoryTeamAccess
	if err := r.Get(ctx, req.NamespacedName, &access); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !access.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &access)
	}

	if !controllerutil.ContainsFinalizer(&access, githubRepositoryTeamAccessFinalizer) {
		controllerutil.AddFinalizer(&access, githubRepositoryTeamAccessFinalizer)
		if err := r.Update(ctx, &access); err != nil {
			return ctrl.Result{}, fmt.Errorf("add GitHubRepositoryTeamAccess finalizer: %w", err)
		}
		return ctrl.Result{}, nil
	}

	resolved, err := resolveRepositoryAccess(
		ctx,
		r.Client,
		r.APIReader,
		r.GitHubClientFactory,
		access.Namespace,
		access.Spec.RepositoryRef,
	)
	if err != nil {
		return r.fail(ctx, &access, nil, "DependencyUnavailable", err)
	}

	if err := verifyRemoteRepository(ctx, resolved); err != nil {
		return r.fail(ctx, &access, resolved, "RepositoryUnavailable", err)
	}

	currentPermission, err := resolved.Client.GetTeamRepositoryPermission(
		ctx,
		resolved.Provider.Spec.Organization,
		access.Spec.TeamSlug,
		resolved.Provider.Spec.Organization,
		resolved.Repository.Spec.Name,
	)
	switch {
	case errors.Is(err, githubclient.ErrNotFound):
		err = resolved.Client.SetTeamRepositoryPermission(
			ctx,
			resolved.Provider.Spec.Organization,
			access.Spec.TeamSlug,
			resolved.Provider.Spec.Organization,
			resolved.Repository.Spec.Name,
			string(access.Spec.Permission),
		)
		if err != nil {
			return r.fail(ctx, &access, resolved, "ReconciliationFailed", fmt.Errorf(
				"configure team %q access to %s/%s: %w",
				access.Spec.TeamSlug,
				resolved.Provider.Spec.Organization,
				resolved.Repository.Spec.Name,
				err,
			))
		}
		currentPermission = string(access.Spec.Permission)
		return r.succeed(ctx, &access, resolved, currentPermission, "AccessConfigured")
	case err != nil:
		return r.fail(ctx, &access, resolved, "ReconciliationFailed", fmt.Errorf(
			"get team %q permission for %s/%s: %w",
			access.Spec.TeamSlug,
			resolved.Provider.Spec.Organization,
			resolved.Repository.Spec.Name,
			err,
		))
	case currentPermission != string(access.Spec.Permission):
		err = resolved.Client.SetTeamRepositoryPermission(
			ctx,
			resolved.Provider.Spec.Organization,
			access.Spec.TeamSlug,
			resolved.Provider.Spec.Organization,
			resolved.Repository.Spec.Name,
			string(access.Spec.Permission),
		)
		if err != nil {
			return r.fail(ctx, &access, resolved, "ReconciliationFailed", fmt.Errorf(
				"update team %q permission for %s/%s: %w",
				access.Spec.TeamSlug,
				resolved.Provider.Spec.Organization,
				resolved.Repository.Spec.Name,
				err,
			))
		}
		currentPermission = string(access.Spec.Permission)
		return r.succeed(ctx, &access, resolved, currentPermission, "AccessUpdated")
	default:
		return r.succeed(ctx, &access, resolved, currentPermission, "AccessAvailable")
	}
}

func (r *GitHubRepositoryTeamAccessReconciler) reconcileDelete(
	ctx context.Context,
	access *githubv1alpha1.GitHubRepositoryTeamAccess,
) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	if !controllerutil.ContainsFinalizer(access, githubRepositoryTeamAccessFinalizer) {
		return ctrl.Result{}, nil
	}

	if access.Spec.EffectiveDeletionPolicy() == githubv1alpha1.RepositoryAccessDeletionPolicyOrphan {
		logger.Info("orphaning GitHub team repository access")
		return ctrl.Result{}, r.removeFinalizer(ctx, access)
	}

	resolved, err := resolveRepositoryAccess(
		ctx,
		r.Client,
		r.APIReader,
		r.GitHubClientFactory,
		access.Namespace,
		access.Spec.RepositoryRef,
	)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("resolve team access dependencies for deletion: %w", err)
	}

	err = resolved.Client.RemoveTeamRepository(
		ctx,
		resolved.Provider.Spec.Organization,
		access.Spec.TeamSlug,
		resolved.Provider.Spec.Organization,
		resolved.Repository.Spec.Name,
	)
	if err != nil && !errors.Is(err, githubclient.ErrNotFound) {
		return ctrl.Result{}, fmt.Errorf(
			"revoke team %q access to %s/%s: %w",
			access.Spec.TeamSlug,
			resolved.Provider.Spec.Organization,
			resolved.Repository.Spec.Name,
			err,
		)
	}

	return ctrl.Result{}, r.removeFinalizer(ctx, access)
}

func (r *GitHubRepositoryTeamAccessReconciler) removeFinalizer(
	ctx context.Context,
	access *githubv1alpha1.GitHubRepositoryTeamAccess,
) error {
	controllerutil.RemoveFinalizer(access, githubRepositoryTeamAccessFinalizer)
	if err := r.Update(ctx, access); err != nil {
		return fmt.Errorf("remove GitHubRepositoryTeamAccess finalizer: %w", err)
	}
	return nil
}

func (r *GitHubRepositoryTeamAccessReconciler) succeed(
	ctx context.Context,
	access *githubv1alpha1.GitHubRepositoryTeamAccess,
	resolved *resolvedRepositoryAccess,
	permission string,
	reason string,
) (ctrl.Result, error) {
	message := fmt.Sprintf(
		"Team %s has %s access to %s/%s",
		access.Spec.TeamSlug,
		permission,
		resolved.Provider.Spec.Organization,
		resolved.Repository.Spec.Name,
	)
	if err := r.setReadyCondition(
		ctx,
		access,
		resolved,
		githubv1alpha1.RepositoryPermission(permission),
		metav1.ConditionTrue,
		reason,
		message,
	); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: repositoryAccessRequeueInterval}, nil
}

func (r *GitHubRepositoryTeamAccessReconciler) fail(
	ctx context.Context,
	access *githubv1alpha1.GitHubRepositoryTeamAccess,
	resolved *resolvedRepositoryAccess,
	reason string,
	reconcileErr error,
) (ctrl.Result, error) {
	if err := r.setReadyCondition(
		ctx,
		access,
		resolved,
		"",
		metav1.ConditionFalse,
		reason,
		reconcileErr.Error(),
	); err != nil {
		return ctrl.Result{}, fmt.Errorf("%w; update status: %v", reconcileErr, err)
	}
	return ctrl.Result{}, reconcileErr
}

func (r *GitHubRepositoryTeamAccessReconciler) setReadyCondition(
	ctx context.Context,
	access *githubv1alpha1.GitHubRepositoryTeamAccess,
	resolved *resolvedRepositoryAccess,
	permission githubv1alpha1.RepositoryPermission,
	status metav1.ConditionStatus,
	reason string,
	message string,
) error {
	previousStatus := access.Status.DeepCopy()
	access.Status.TeamSlug = access.Spec.TeamSlug
	access.Status.ObservedGeneration = access.Generation
	if resolved != nil {
		access.Status.ProviderConfigRef = resolved.Provider.Name
		access.Status.Organization = resolved.Provider.Spec.Organization
		access.Status.Repository = resolved.Repository.Spec.Name
	}
	if permission != "" {
		access.Status.Permission = permission
	}

	meta.SetStatusCondition(&access.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             status,
		ObservedGeneration: access.Generation,
		Reason:             reason,
		Message:            message,
	})

	if apiequality.Semantic.DeepEqual(previousStatus, &access.Status) {
		return nil
	}
	return r.Status().Update(ctx, access)
}

// SetupWithManager sets up the controller with the Manager.
func (r *GitHubRepositoryTeamAccessReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&githubv1alpha1.GitHubRepositoryTeamAccess{}).
		Named("githubrepositoryteamaccess").
		Complete(r)
}
