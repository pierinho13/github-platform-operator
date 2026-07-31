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

const githubTeamMembershipFinalizer = "github.k8sready.com/team-membership-finalizer"

type resolvedTeamMembership struct {
	Organization *resolvedOrganization
	Team         *githubv1alpha1.GitHubTeam
	RemoteTeam   *githubclient.Team
}

// GitHubTeamMembershipReconciler reconciles membership in managed teams.
type GitHubTeamMembershipReconciler struct {
	client.Client
	APIReader           client.Reader
	Scheme              *runtime.Scheme
	GitHubClientFactory githubclient.OrganizationClientFactory
	GitHubTokenProvider githubclient.TokenProvider
}

// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubteammemberships,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubteammemberships/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubteammemberships/finalizers,verbs=update
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubteams;githubproviderconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get

// Reconcile adds users to teams and updates their role.
func (r *GitHubTeamMembershipReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	var resource githubv1alpha1.GitHubTeamMembership
	if err := r.Get(ctx, req.NamespacedName, &resource); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !resource.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &resource)
	}
	if !controllerutil.ContainsFinalizer(&resource, githubTeamMembershipFinalizer) {
		controllerutil.AddFinalizer(&resource, githubTeamMembershipFinalizer)
		if err := r.Update(ctx, &resource); err != nil {
			return ctrl.Result{}, fmt.Errorf("add GitHubTeamMembership finalizer: %w", err)
		}
		return ctrl.Result{}, nil
	}

	resolved, err := r.resolve(ctx, &resource, false)
	if err != nil {
		return r.fail(ctx, &resource, nil, dependencyFailureReason(err), err)
	}

	membership, err := resolved.Organization.Client.GetTeamMembership(
		ctx,
		resolved.Organization.Provider.Spec.Organization,
		resolved.RemoteTeam.Slug,
		resource.Spec.Username,
	)
	reason := "TeamMembershipAvailable"
	switch {
	case errors.Is(err, githubclient.ErrNotFound):
		membership, err = resolved.Organization.Client.SetTeamMembership(
			ctx,
			resolved.Organization.Provider.Spec.Organization,
			resolved.RemoteTeam.Slug,
			resource.Spec.Username,
			string(resource.Spec.Role),
		)
		reason = "TeamMembershipConfigured"
	case err == nil && membership.Role != string(resource.Spec.Role):
		membership, err = resolved.Organization.Client.SetTeamMembership(
			ctx,
			resolved.Organization.Provider.Spec.Organization,
			resolved.RemoteTeam.Slug,
			resource.Spec.Username,
			string(resource.Spec.Role),
		)
		reason = "TeamMembershipUpdated"
	}
	if err != nil {
		return r.fail(ctx, &resource, resolved, "ReconciliationFailed", fmt.Errorf(
			"reconcile %s membership for %s in team %s/%s: %w",
			resource.Spec.Role,
			resource.Spec.Username,
			resolved.Organization.Provider.Spec.Organization,
			resolved.RemoteTeam.Slug,
			err,
		))
	}
	return r.finish(ctx, &resource, resolved, membership, reason)
}

func (r *GitHubTeamMembershipReconciler) resolve(
	ctx context.Context,
	resource *githubv1alpha1.GitHubTeamMembership,
	allowStatusFallback bool,
) (*resolvedTeamMembership, error) {
	var team githubv1alpha1.GitHubTeam
	err := r.Get(ctx, types.NamespacedName{
		Namespace: resource.Namespace,
		Name:      resource.Spec.TeamRef.Name,
	}, &team)
	if err != nil {
		if !allowStatusFallback || resource.Status.ProviderConfigRef == "" || resource.Status.TeamSlug == "" {
			return nil, fmt.Errorf(
				"get GitHubTeam %s/%s: %w",
				resource.Namespace,
				resource.Spec.TeamRef.Name,
				err,
			)
		}
		resolved, resolveErr := resolveOrganization(
			ctx,
			r.Client,
			r.APIReader,
			r.GitHubClientFactory,
			r.GitHubTokenProvider,
			resource.Status.ProviderConfigRef,
		)
		if resolveErr != nil {
			return nil, resolveErr
		}
		remote := &githubclient.Team{Slug: resource.Status.TeamSlug}
		return &resolvedTeamMembership{Organization: resolved, RemoteTeam: remote}, nil
	}

	resolved, err := resolveOrganization(
		ctx,
		r.Client,
		r.APIReader,
		r.GitHubClientFactory,
		r.GitHubTokenProvider,
		team.Spec.EffectiveProviderConfigRef(),
	)
	if err != nil {
		return nil, err
	}
	remote, err := findRemoteTeam(
		ctx,
		resolved.Client,
		resolved.Provider.Spec.Organization,
		&team,
	)
	if err != nil {
		if errors.Is(err, githubclient.ErrNotFound) {
			return nil, fmt.Errorf(
				"GitHub team %s/%s does not exist",
				resolved.Provider.Spec.Organization,
				team.Spec.Name,
			)
		}
		return nil, err
	}
	return &resolvedTeamMembership{Organization: resolved, Team: &team, RemoteTeam: remote}, nil
}

func (r *GitHubTeamMembershipReconciler) finish(
	ctx context.Context,
	resource *githubv1alpha1.GitHubTeamMembership,
	resolved *resolvedTeamMembership,
	membership *githubclient.Membership,
	reason string,
) (ctrl.Result, error) {
	status := metav1.ConditionTrue
	message := fmt.Sprintf(
		"GitHub user %s is a %s of team %s/%s",
		resource.Spec.Username,
		membership.Role,
		resolved.Organization.Provider.Spec.Organization,
		resolved.RemoteTeam.Slug,
	)
	if membership.State == organizationMembershipStatePending {
		status = metav1.ConditionFalse
		reason = invitationPendingReason
		message = fmt.Sprintf(
			"GitHub user %s has pending membership in team %s/%s",
			resource.Spec.Username,
			resolved.Organization.Provider.Spec.Organization,
			resolved.RemoteTeam.Slug,
		)
	}
	if err := r.setReadyCondition(ctx, resource, resolved, membership, status, reason, message); err != nil {
		return ctrl.Result{}, fmt.Errorf("update GitHubTeamMembership status: %w", err)
	}
	return ctrl.Result{RequeueAfter: organizationRequeueAfter}, nil
}

func (r *GitHubTeamMembershipReconciler) reconcileDelete(
	ctx context.Context,
	resource *githubv1alpha1.GitHubTeamMembership,
) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	if !controllerutil.ContainsFinalizer(resource, githubTeamMembershipFinalizer) {
		return ctrl.Result{}, nil
	}
	if resource.Spec.EffectiveDeletionPolicy() == githubv1alpha1.GitHubMembershipDeletionPolicyOrphan {
		logger.Info("orphaning GitHub team membership")
		return ctrl.Result{}, r.removeFinalizer(ctx, resource)
	}

	resolved, err := r.resolve(ctx, resource, true)
	if err != nil {
		if result, ok := githubDeferredResult(err); ok {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolve GitHubTeamMembership for deletion: %w", err)
	}
	err = resolved.Organization.Client.RemoveTeamMembership(
		ctx,
		resolved.Organization.Provider.Spec.Organization,
		resolved.RemoteTeam.Slug,
		resource.Spec.Username,
	)
	if err != nil && !errors.Is(err, githubclient.ErrNotFound) {
		if result, ok := githubDeferredResult(err); ok {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf(
			"remove GitHub user %s from team %s/%s: %w",
			resource.Spec.Username,
			resolved.Organization.Provider.Spec.Organization,
			resolved.RemoteTeam.Slug,
			err,
		)
	}
	return ctrl.Result{}, r.removeFinalizer(ctx, resource)
}

func (r *GitHubTeamMembershipReconciler) removeFinalizer(
	ctx context.Context,
	resource *githubv1alpha1.GitHubTeamMembership,
) error {
	controllerutil.RemoveFinalizer(resource, githubTeamMembershipFinalizer)
	if err := r.Update(ctx, resource); err != nil {
		return fmt.Errorf("remove GitHubTeamMembership finalizer: %w", err)
	}
	return nil
}

func (r *GitHubTeamMembershipReconciler) fail(
	ctx context.Context,
	resource *githubv1alpha1.GitHubTeamMembership,
	resolved *resolvedTeamMembership,
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

func (r *GitHubTeamMembershipReconciler) setReadyCondition(
	ctx context.Context,
	resource *githubv1alpha1.GitHubTeamMembership,
	resolved *resolvedTeamMembership,
	membership *githubclient.Membership,
	status metav1.ConditionStatus,
	reason string,
	message string,
) error {
	previousStatus := resource.Status.DeepCopy()
	resource.Status.Team = resource.Spec.TeamRef.Name
	resource.Status.Username = resource.Spec.Username
	resource.Status.ObservedGeneration = resource.Generation
	if resolved != nil {
		resource.Status.ProviderConfigRef = resolved.Organization.Provider.Name
		resource.Status.Organization = resolved.Organization.Provider.Spec.Organization
		resource.Status.TeamSlug = resolved.RemoteTeam.Slug
	}
	if membership != nil {
		resource.Status.Role = githubv1alpha1.GitHubTeamMembershipRole(membership.Role)
		resource.Status.State = membership.State
		resource.Status.InvitationPending = membership.State == organizationMembershipStatePending
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
func (r *GitHubTeamMembershipReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&githubv1alpha1.GitHubTeamMembership{}).
		Named("githubteammembership").
		Complete(r)
}
