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

const githubTeamFinalizer = "github.k8sready.com/team-finalizer"

// GitHubTeamReconciler reconciles organization teams.
type GitHubTeamReconciler struct {
	client.Client
	APIReader              client.Reader
	Scheme                 *runtime.Scheme
	GitHubClientFactory    githubclient.OrganizationClientFactory
	GitHubTokenProvider    githubclient.TokenProvider
	DriftDetectionInterval time.Duration
}

// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubteams,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubteams/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubteams/finalizers,verbs=update
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubteammemberships;githubproviderconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get

// Reconcile creates, adopts and updates a GitHub organization team.
func (r *GitHubTeamReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	var resource githubv1alpha1.GitHubTeam
	if err := r.Get(ctx, req.NamespacedName, &resource); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !resource.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &resource)
	}
	if !controllerutil.ContainsFinalizer(&resource, githubTeamFinalizer) {
		controllerutil.AddFinalizer(&resource, githubTeamFinalizer)
		if err := r.Update(ctx, &resource); err != nil {
			return ctrl.Result{}, fmt.Errorf("add GitHubTeam finalizer: %w", err)
		}
		return ctrl.Result{}, nil
	}

	resolved, err := resolveOrganization(
		ctx,
		r.Client,
		r.APIReader,
		r.GitHubClientFactory,
		r.GitHubTokenProvider,
		resource.Spec.EffectiveProviderConfigRef(),
	)
	if err != nil {
		return r.fail(ctx, &resource, nil, dependencyFailureReason(err), err)
	}

	remote, err := findRemoteTeam(
		ctx,
		resolved.Client,
		resolved.Provider.Spec.Organization,
		&resource,
	)
	reason := "TeamAvailable"
	switch {
	case errors.Is(err, githubclient.ErrNotFound):
		description := ""
		if resource.Spec.Description != nil {
			description = *resource.Spec.Description
		}
		remote, err = resolved.Client.CreateTeam(
			ctx,
			resolved.Provider.Spec.Organization,
			githubclient.TeamCreate{
				Name:        resource.Spec.Name,
				Description: description,
				Privacy:     string(resource.Spec.EffectivePrivacyForCreation()),
			},
		)
		if err != nil {
			return r.fail(ctx, &resource, resolved, "ReconciliationFailed", fmt.Errorf(
				"create GitHub team %q in %s: %w",
				resource.Spec.Name,
				resolved.Provider.Spec.Organization,
				err,
			))
		}
		reason = "TeamCreated"
	case err != nil:
		return r.fail(ctx, &resource, resolved, "ReconciliationFailed", fmt.Errorf(
			"find GitHub team %q in %s: %w",
			resource.Spec.Name,
			resolved.Provider.Spec.Organization,
			err,
		))
	}

	update := desiredTeamUpdate(resource.Spec, remote)
	if !update.Empty() {
		remote, err = resolved.Client.UpdateTeam(
			ctx,
			resolved.Provider.Spec.Organization,
			remote.Slug,
			update,
		)
		if err != nil {
			return r.fail(ctx, &resource, resolved, "ReconciliationFailed", fmt.Errorf(
				"update GitHub team %q in %s: %w",
				resource.Spec.Name,
				resolved.Provider.Spec.Organization,
				err,
			))
		}
		if reason != "TeamCreated" {
			reason = "TeamUpdated"
		}
	}

	if err := r.setReadyCondition(
		ctx,
		&resource,
		resolved,
		remote,
		metav1.ConditionTrue,
		reason,
		fmt.Sprintf(
			"GitHub team %s/%s is synchronized as %s",
			resolved.Provider.Spec.Organization,
			remote.Name,
			remote.Slug,
		),
	); err != nil {
		return ctrl.Result{}, fmt.Errorf("update GitHubTeam status: %w", err)
	}
	return driftDetectionResult(r.DriftDetectionInterval), nil
}

func desiredTeamUpdate(
	spec githubv1alpha1.GitHubTeamSpec,
	remote *githubclient.Team,
) githubclient.TeamUpdate {
	var update githubclient.TeamUpdate
	if spec.Description != nil && remote.Description != *spec.Description {
		update.Description = spec.Description
	}
	if spec.Privacy != nil && remote.Privacy != string(*spec.Privacy) {
		privacy := string(*spec.Privacy)
		update.Privacy = &privacy
	}
	return update
}

func (r *GitHubTeamReconciler) reconcileDelete(
	ctx context.Context,
	resource *githubv1alpha1.GitHubTeam,
) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	if !controllerutil.ContainsFinalizer(resource, githubTeamFinalizer) {
		return ctrl.Result{}, nil
	}
	if resource.Spec.EffectiveDeletionPolicy() == githubv1alpha1.GitHubTeamDeletionPolicyOrphan {
		logger.Info("orphaning GitHub team")
		return ctrl.Result{}, r.removeFinalizer(ctx, resource)
	}

	dependency, err := r.findDependency(ctx, resource)
	if err != nil {
		return ctrl.Result{}, err
	}
	if dependency != "" {
		message := fmt.Sprintf("GitHubTeam is still referenced by %s", dependency)
		if err := r.setReadyCondition(
			ctx,
			resource,
			nil,
			nil,
			metav1.ConditionFalse,
			"TeamInUse",
			message,
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("update GitHubTeam deletion status: %w", err)
		}
		return driftDetectionResult(r.DriftDetectionInterval), nil
	}

	resolved, err := resolveOrganization(
		ctx,
		r.Client,
		r.APIReader,
		r.GitHubClientFactory,
		r.GitHubTokenProvider,
		resource.Spec.EffectiveProviderConfigRef(),
	)
	if err != nil {
		if result, ok := githubDeferredResult(err); ok {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolve GitHubTeam provider for deletion: %w", err)
	}
	remote, err := findRemoteTeam(
		ctx,
		resolved.Client,
		resolved.Provider.Spec.Organization,
		resource,
	)
	if err != nil && !errors.Is(err, githubclient.ErrNotFound) {
		if result, ok := githubDeferredResult(err); ok {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf("find GitHubTeam before deletion: %w", err)
	}
	if remote != nil {
		err = resolved.Client.DeleteTeam(ctx, resolved.Provider.Spec.Organization, remote.Slug)
		if err != nil && !errors.Is(err, githubclient.ErrNotFound) {
			if result, ok := githubDeferredResult(err); ok {
				return result, nil
			}
			return ctrl.Result{}, fmt.Errorf("delete GitHub team %s/%s: %w", resolved.Provider.Spec.Organization, remote.Slug, err)
		}
	}
	return ctrl.Result{}, r.removeFinalizer(ctx, resource)
}

func (r *GitHubTeamReconciler) findDependency(
	ctx context.Context,
	resource *githubv1alpha1.GitHubTeam,
) (string, error) {
	var memberships githubv1alpha1.GitHubTeamMembershipList
	if err := r.List(ctx, &memberships, client.InNamespace(resource.Namespace)); err != nil {
		return "", fmt.Errorf("list GitHubTeamMemberships: %w", err)
	}
	for i := range memberships.Items {
		if memberships.Items[i].Spec.TeamRef.Name == resource.Name {
			return fmt.Sprintf(
				"GitHubTeamMembership %s/%s",
				memberships.Items[i].Namespace,
				memberships.Items[i].Name,
			), nil
		}
	}
	return "", nil
}

func (r *GitHubTeamReconciler) removeFinalizer(
	ctx context.Context,
	resource *githubv1alpha1.GitHubTeam,
) error {
	controllerutil.RemoveFinalizer(resource, githubTeamFinalizer)
	if err := r.Update(ctx, resource); err != nil {
		return fmt.Errorf("remove GitHubTeam finalizer: %w", err)
	}
	return nil
}

func (r *GitHubTeamReconciler) fail(
	ctx context.Context,
	resource *githubv1alpha1.GitHubTeam,
	resolved *resolvedOrganization,
	reason string,
	reconcileErr error,
) (ctrl.Result, error) {
	if err := r.setReadyCondition(
		ctx,
		resource,
		resolved,
		nil,
		metav1.ConditionFalse,
		reason,
		reconcileErr.Error(),
	); err != nil {
		return ctrl.Result{}, fmt.Errorf("%w; update status: %v", reconcileErr, err)
	}
	if result, ok := githubDeferredResult(reconcileErr); ok {
		return result, nil
	}
	return ctrl.Result{}, reconcileErr
}

func (r *GitHubTeamReconciler) setReadyCondition(
	ctx context.Context,
	resource *githubv1alpha1.GitHubTeam,
	resolved *resolvedOrganization,
	remote *githubclient.Team,
	status metav1.ConditionStatus,
	reason string,
	message string,
) error {
	previousStatus := resource.Status.DeepCopy()
	resource.Status.ObservedGeneration = resource.Generation
	if resolved != nil {
		resource.Status.ProviderConfigRef = resolved.Provider.Name
		resource.Status.Organization = resolved.Provider.Spec.Organization
	}
	if remote != nil {
		resource.Status.TeamID = remote.ID
		resource.Status.Name = remote.Name
		resource.Status.Slug = remote.Slug
		resource.Status.Description = remote.Description
		resource.Status.Privacy = githubv1alpha1.GitHubTeamPrivacy(remote.Privacy)
	}
	meta.SetStatusCondition(&resource.Status.Conditions, metav1.Condition{
		Type:               conditionTypeReady,
		Status:             status,
		ObservedGeneration: resource.Generation,
		Reason:             reason,
		Message:            message,
	})
	if apiequality.Semantic.DeepEqual(previousStatus, &resource.Status) {
		return nil
	}
	return r.Status().Update(ctx, resource)
}

// SetupWithManager sets up the controller with the Manager.
func (r *GitHubTeamReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&githubv1alpha1.GitHubTeam{}).
		Named("githubteam").
		Complete(r)
}
