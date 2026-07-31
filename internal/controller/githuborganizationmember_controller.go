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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	githubv1alpha1 "github.com/pierinho13/github-platform-operator/api/v1alpha1"
	githubclient "github.com/pierinho13/github-platform-operator/internal/github"
)

const githubOrganizationMemberFinalizer = "github.k8sready.com/organization-member-finalizer"

// GitHubOrganizationMemberReconciler reconciles direct organization membership.
type GitHubOrganizationMemberReconciler struct {
	client.Client
	APIReader           client.Reader
	Scheme              *runtime.Scheme
	GitHubClientFactory githubclient.OrganizationClientFactory
	GitHubTokenProvider githubclient.TokenProvider
}

// +kubebuilder:rbac:groups=github.k8sready.com,resources=githuborganizationmembers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githuborganizationmembers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githuborganizationmembers/finalizers,verbs=update
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubproviderconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get

// Reconcile invites organization members and manages their standard role.
func (r *GitHubOrganizationMemberReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	var resource githubv1alpha1.GitHubOrganizationMember
	if err := r.Get(ctx, req.NamespacedName, &resource); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !resource.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &resource)
	}
	if !controllerutil.ContainsFinalizer(&resource, githubOrganizationMemberFinalizer) {
		controllerutil.AddFinalizer(&resource, githubOrganizationMemberFinalizer)
		if err := r.Update(ctx, &resource); err != nil {
			return ctrl.Result{}, fmt.Errorf("add GitHubOrganizationMember finalizer: %w", err)
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

	membership, err := resolved.Client.GetOrganizationMembership(
		ctx,
		resolved.Provider.Spec.Organization,
		resource.Spec.Username,
	)
	reason := "OrganizationMembershipAvailable"
	switch {
	case errors.Is(err, githubclient.ErrNotFound):
		membership, err = resolved.Client.SetOrganizationMembership(
			ctx,
			resolved.Provider.Spec.Organization,
			resource.Spec.Username,
			string(resource.Spec.Role),
		)
		reason = "OrganizationMembershipConfigured"
	case err == nil && membership.Role != string(resource.Spec.Role):
		membership, err = resolved.Client.SetOrganizationMembership(
			ctx,
			resolved.Provider.Spec.Organization,
			resource.Spec.Username,
			string(resource.Spec.Role),
		)
		reason = "OrganizationMembershipUpdated"
	}
	if err != nil {
		return r.fail(ctx, &resource, resolved, "ReconciliationFailed", fmt.Errorf(
			"reconcile %s membership for %s in organization %s: %w",
			resource.Spec.Role,
			resource.Spec.Username,
			resolved.Provider.Spec.Organization,
			err,
		))
	}
	return r.finish(ctx, &resource, resolved, membership, reason)
}

func (r *GitHubOrganizationMemberReconciler) finish(
	ctx context.Context,
	resource *githubv1alpha1.GitHubOrganizationMember,
	resolved *resolvedOrganization,
	membership *githubclient.Membership,
	reason string,
) (ctrl.Result, error) {
	status := metav1.ConditionTrue
	message := fmt.Sprintf(
		"GitHub user %s has %s membership in organization %s",
		resource.Spec.Username,
		membership.Role,
		resolved.Provider.Spec.Organization,
	)
	if membership.State == organizationMembershipStatePending {
		status = metav1.ConditionFalse
		reason = invitationPendingReason
		message = fmt.Sprintf(
			"GitHub user %s has a pending invitation to organization %s",
			resource.Spec.Username,
			resolved.Provider.Spec.Organization,
		)
	}
	if err := r.setReadyCondition(ctx, resource, resolved, membership, status, reason, message); err != nil {
		return ctrl.Result{}, fmt.Errorf("update GitHubOrganizationMember status: %w", err)
	}
	return ctrl.Result{RequeueAfter: organizationRequeueAfter}, nil
}

func (r *GitHubOrganizationMemberReconciler) reconcileDelete(
	ctx context.Context,
	resource *githubv1alpha1.GitHubOrganizationMember,
) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	if !controllerutil.ContainsFinalizer(resource, githubOrganizationMemberFinalizer) {
		return ctrl.Result{}, nil
	}
	if resource.Spec.EffectiveDeletionPolicy() == githubv1alpha1.GitHubMembershipDeletionPolicyOrphan {
		logger.Info("orphaning GitHub organization membership")
		return ctrl.Result{}, r.removeFinalizer(ctx, resource)
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
		return ctrl.Result{}, fmt.Errorf("resolve GitHubOrganizationMember for deletion: %w", err)
	}
	err = resolved.Client.RemoveOrganizationMembership(
		ctx,
		resolved.Provider.Spec.Organization,
		resource.Spec.Username,
	)
	if err != nil && !errors.Is(err, githubclient.ErrNotFound) {
		if result, ok := githubDeferredResult(err); ok {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf(
			"remove GitHub user %s from organization %s: %w",
			resource.Spec.Username,
			resolved.Provider.Spec.Organization,
			err,
		)
	}
	return ctrl.Result{}, r.removeFinalizer(ctx, resource)
}

func (r *GitHubOrganizationMemberReconciler) removeFinalizer(
	ctx context.Context,
	resource *githubv1alpha1.GitHubOrganizationMember,
) error {
	controllerutil.RemoveFinalizer(resource, githubOrganizationMemberFinalizer)
	if err := r.Update(ctx, resource); err != nil {
		return fmt.Errorf("remove GitHubOrganizationMember finalizer: %w", err)
	}
	return nil
}

func (r *GitHubOrganizationMemberReconciler) fail(
	ctx context.Context,
	resource *githubv1alpha1.GitHubOrganizationMember,
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

func (r *GitHubOrganizationMemberReconciler) setReadyCondition(
	ctx context.Context,
	resource *githubv1alpha1.GitHubOrganizationMember,
	resolved *resolvedOrganization,
	membership *githubclient.Membership,
	status metav1.ConditionStatus,
	reason string,
	message string,
) error {
	previousStatus := resource.Status.DeepCopy()
	resource.Status.Username = resource.Spec.Username
	resource.Status.ObservedGeneration = resource.Generation
	if resolved != nil {
		resource.Status.ProviderConfigRef = resolved.Provider.Name
		resource.Status.Organization = resolved.Provider.Spec.Organization
	}
	if membership != nil {
		resource.Status.Role = githubv1alpha1.GitHubOrganizationRole(membership.Role)
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
func (r *GitHubOrganizationMemberReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&githubv1alpha1.GitHubOrganizationMember{}).
		Named("githuborganizationmember").
		Complete(r)
}
