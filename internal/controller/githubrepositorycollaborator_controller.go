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

const githubRepositoryCollaboratorFinalizer = "github.k8sready.com/collaborator-finalizer"

// GitHubRepositoryCollaboratorReconciler reconciles direct user repository access.
type GitHubRepositoryCollaboratorReconciler struct {
	client.Client
	APIReader           client.Reader
	Scheme              *runtime.Scheme
	GitHubClientFactory githubclient.RepositoryAccessClientFactory
	GitHubTokenProvider githubclient.TokenProvider
}

// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubrepositorycollaborators,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubrepositorycollaborators/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubrepositorycollaborators/finalizers,verbs=update
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubrepositories;githubproviderconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get

func (r *GitHubRepositoryCollaboratorReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	var collaborator githubv1alpha1.GitHubRepositoryCollaborator
	if err := r.Get(ctx, req.NamespacedName, &collaborator); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !collaborator.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &collaborator)
	}

	if !controllerutil.ContainsFinalizer(&collaborator, githubRepositoryCollaboratorFinalizer) {
		controllerutil.AddFinalizer(&collaborator, githubRepositoryCollaboratorFinalizer)
		if err := r.Update(ctx, &collaborator); err != nil {
			return ctrl.Result{}, fmt.Errorf("add GitHubRepositoryCollaborator finalizer: %w", err)
		}
		return ctrl.Result{}, nil
	}

	resolved, err := resolveRepositoryAccess(
		ctx,
		r.Client,
		r.APIReader,
		r.GitHubClientFactory,
		r.GitHubTokenProvider,
		collaborator.Namespace,
		collaborator.Spec.RepositoryRef,
	)
	if err != nil {
		return r.fail(ctx, &collaborator, nil, dependencyFailureReason(err), err)
	}
	remoteRepository, err := getRemoteRepository(ctx, resolved)
	if err != nil {
		return r.fail(ctx, &collaborator, resolved, "RepositoryUnavailable", err)
	}

	currentAccess, err := resolved.Client.GetCollaboratorAccess(
		ctx,
		resolved.Provider.Spec.Organization,
		resolved.Repository.Spec.Name,
		collaborator.Spec.Username,
	)
	switch {
	case errors.Is(err, githubclient.ErrNotFound):
		if remoteRepository.Archived {
			return r.pauseForArchivedRepository(ctx, &collaborator, resolved, nil)
		}
		currentAccess, err = resolved.Client.SetCollaboratorPermission(
			ctx,
			resolved.Provider.Spec.Organization,
			resolved.Repository.Spec.Name,
			collaborator.Spec.Username,
			string(collaborator.Spec.Permission),
		)
		if err != nil {
			if isArchivedRepositoryError(err) {
				return r.pauseForArchivedRepository(ctx, &collaborator, resolved, nil)
			}
			return r.fail(ctx, &collaborator, resolved, "ReconciliationFailed", fmt.Errorf(
				"configure collaborator %q access to %s/%s: %w",
				collaborator.Spec.Username,
				resolved.Provider.Spec.Organization,
				resolved.Repository.Spec.Name,
				err,
			))
		}
		return r.finishAccess(ctx, &collaborator, resolved, currentAccess, "AccessConfigured")
	case isArchivedRepositoryError(err):
		return r.pauseForArchivedRepository(ctx, &collaborator, resolved, nil)
	case err != nil:
		return r.fail(ctx, &collaborator, resolved, "ReconciliationFailed", fmt.Errorf(
			"get collaborator %q access to %s/%s: %w",
			collaborator.Spec.Username,
			resolved.Provider.Spec.Organization,
			resolved.Repository.Spec.Name,
			err,
		))
	case currentAccess.Permission != string(collaborator.Spec.Permission) && currentAccess.InvitationPending:
		if remoteRepository.Archived {
			return r.pauseForArchivedRepository(ctx, &collaborator, resolved, currentAccess)
		}
		currentAccess, err = resolved.Client.UpdateRepositoryInvitation(
			ctx,
			resolved.Provider.Spec.Organization,
			resolved.Repository.Spec.Name,
			currentAccess.InvitationID,
			string(collaborator.Spec.Permission),
		)
		if err != nil {
			if isArchivedRepositoryError(err) {
				return r.pauseForArchivedRepository(ctx, &collaborator, resolved, currentAccess)
			}
			return r.fail(ctx, &collaborator, resolved, "ReconciliationFailed", fmt.Errorf(
				"update invitation for collaborator %q on %s/%s: %w",
				collaborator.Spec.Username,
				resolved.Provider.Spec.Organization,
				resolved.Repository.Spec.Name,
				err,
			))
		}
		return r.finishAccess(ctx, &collaborator, resolved, currentAccess, "InvitationUpdated")
	case currentAccess.Permission != string(collaborator.Spec.Permission):
		if remoteRepository.Archived {
			return r.pauseForArchivedRepository(ctx, &collaborator, resolved, currentAccess)
		}
		currentAccess, err = resolved.Client.SetCollaboratorPermission(
			ctx,
			resolved.Provider.Spec.Organization,
			resolved.Repository.Spec.Name,
			collaborator.Spec.Username,
			string(collaborator.Spec.Permission),
		)
		if err != nil {
			if isArchivedRepositoryError(err) {
				return r.pauseForArchivedRepository(ctx, &collaborator, resolved, currentAccess)
			}
			return r.fail(ctx, &collaborator, resolved, "ReconciliationFailed", fmt.Errorf(
				"update collaborator %q access to %s/%s: %w",
				collaborator.Spec.Username,
				resolved.Provider.Spec.Organization,
				resolved.Repository.Spec.Name,
				err,
			))
		}
		return r.finishAccess(ctx, &collaborator, resolved, currentAccess, "AccessUpdated")
	default:
		return r.finishAccess(ctx, &collaborator, resolved, currentAccess, "AccessAvailable")
	}
}

func (r *GitHubRepositoryCollaboratorReconciler) finishAccess(
	ctx context.Context,
	collaborator *githubv1alpha1.GitHubRepositoryCollaborator,
	resolved *resolvedRepositoryAccess,
	access *githubclient.CollaboratorAccess,
	reason string,
) (ctrl.Result, error) {
	conditionStatus := metav1.ConditionTrue
	message := fmt.Sprintf(
		"Collaborator %s has %s access to %s/%s",
		collaborator.Spec.Username,
		access.Permission,
		resolved.Provider.Spec.Organization,
		resolved.Repository.Spec.Name,
	)
	if access.InvitationPending {
		conditionStatus = metav1.ConditionFalse
		reason = "InvitationPending"
		message = fmt.Sprintf(
			"Collaborator %s has a pending %s invitation to %s/%s",
			collaborator.Spec.Username,
			access.Permission,
			resolved.Provider.Spec.Organization,
			resolved.Repository.Spec.Name,
		)
	}

	if err := r.setReadyCondition(
		ctx,
		collaborator,
		resolved,
		access,
		conditionStatus,
		reason,
		message,
	); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: repositoryAccessRequeueInterval}, nil
}

func (r *GitHubRepositoryCollaboratorReconciler) pauseForArchivedRepository(
	ctx context.Context,
	collaborator *githubv1alpha1.GitHubRepositoryCollaborator,
	resolved *resolvedRepositoryAccess,
	access *githubclient.CollaboratorAccess,
) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info(
		"repository is archived; skipping collaborator reconciliation",
		"organization", resolved.Provider.Spec.Organization,
		"repository", resolved.Repository.Spec.Name,
		"username", collaborator.Spec.Username,
	)

	message := fmt.Sprintf(
		"GitHub repository %s/%s is archived and read-only; collaborator reconciliation is paused",
		resolved.Provider.Spec.Organization,
		resolved.Repository.Spec.Name,
	)
	if err := r.setReadyCondition(
		ctx,
		collaborator,
		resolved,
		access,
		metav1.ConditionFalse,
		"RepositoryArchived",
		message,
	); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: repositoryAccessRequeueInterval}, nil
}

func (r *GitHubRepositoryCollaboratorReconciler) reconcileDelete(
	ctx context.Context,
	collaborator *githubv1alpha1.GitHubRepositoryCollaborator,
) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	if !controllerutil.ContainsFinalizer(collaborator, githubRepositoryCollaboratorFinalizer) {
		return ctrl.Result{}, nil
	}

	if collaborator.Spec.EffectiveDeletionPolicy() == githubv1alpha1.RepositoryAccessDeletionPolicyOrphan {
		logger.Info("orphaning GitHub collaborator access")
		return ctrl.Result{}, r.removeFinalizer(ctx, collaborator)
	}

	resolved, err := resolveRepositoryAccess(
		ctx,
		r.Client,
		r.APIReader,
		r.GitHubClientFactory,
		r.GitHubTokenProvider,
		collaborator.Namespace,
		collaborator.Spec.RepositoryRef,
	)
	if err != nil {
		if result, ok := githubDeferredResult(err); ok {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolve collaborator dependencies for deletion: %w", err)
	}

	currentAccess, err := resolved.Client.GetCollaboratorAccess(
		ctx,
		resolved.Provider.Spec.Organization,
		resolved.Repository.Spec.Name,
		collaborator.Spec.Username,
	)
	if err != nil && !errors.Is(err, githubclient.ErrNotFound) {
		if result, ok := githubDeferredResult(err); ok {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf(
			"get collaborator %q access before deletion: %w",
			collaborator.Spec.Username,
			err,
		)
	}

	invitationID := int64(0)
	if currentAccess != nil && currentAccess.InvitationPending {
		invitationID = currentAccess.InvitationID
	} else if collaborator.Status.InvitationPending {
		invitationID = collaborator.Status.InvitationID
	}

	if resolved.Repository.Status.Archived {
		logger.Info(
			"repository is archived; deferring collaborator revocation",
			"organization", resolved.Provider.Spec.Organization,
			"repository", resolved.Repository.Spec.Name,
			"username", collaborator.Spec.Username,
		)
		return ctrl.Result{RequeueAfter: repositoryAccessRequeueInterval}, nil
	}

	err = resolved.Client.RemoveCollaboratorAccess(
		ctx,
		resolved.Provider.Spec.Organization,
		resolved.Repository.Spec.Name,
		collaborator.Spec.Username,
		invitationID,
	)
	if err != nil && !errors.Is(err, githubclient.ErrNotFound) {
		if isArchivedRepositoryError(err) {
			logger.Info(
				"repository became archived while revoking collaborator access; deferring revocation",
				"organization", resolved.Provider.Spec.Organization,
				"repository", resolved.Repository.Spec.Name,
				"username", collaborator.Spec.Username,
			)
			return ctrl.Result{RequeueAfter: repositoryAccessRequeueInterval}, nil
		}
		if result, ok := githubDeferredResult(err); ok {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf(
			"revoke collaborator %q access to %s/%s: %w",
			collaborator.Spec.Username,
			resolved.Provider.Spec.Organization,
			resolved.Repository.Spec.Name,
			err,
		)
	}

	return ctrl.Result{}, r.removeFinalizer(ctx, collaborator)
}

func (r *GitHubRepositoryCollaboratorReconciler) removeFinalizer(
	ctx context.Context,
	collaborator *githubv1alpha1.GitHubRepositoryCollaborator,
) error {
	controllerutil.RemoveFinalizer(collaborator, githubRepositoryCollaboratorFinalizer)
	if err := r.Update(ctx, collaborator); err != nil {
		return fmt.Errorf("remove GitHubRepositoryCollaborator finalizer: %w", err)
	}
	return nil
}

func (r *GitHubRepositoryCollaboratorReconciler) fail(
	ctx context.Context,
	collaborator *githubv1alpha1.GitHubRepositoryCollaborator,
	resolved *resolvedRepositoryAccess,
	reason string,
	reconcileErr error,
) (ctrl.Result, error) {
	if err := r.setReadyCondition(
		ctx,
		collaborator,
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

func (r *GitHubRepositoryCollaboratorReconciler) setReadyCondition(
	ctx context.Context,
	collaborator *githubv1alpha1.GitHubRepositoryCollaborator,
	resolved *resolvedRepositoryAccess,
	access *githubclient.CollaboratorAccess,
	status metav1.ConditionStatus,
	reason string,
	message string,
) error {
	previousStatus := collaborator.Status.DeepCopy()
	collaborator.Status.Username = collaborator.Spec.Username
	collaborator.Status.ObservedGeneration = collaborator.Generation
	if resolved != nil {
		collaborator.Status.ProviderConfigRef = resolved.Provider.Name
		collaborator.Status.Organization = resolved.Provider.Spec.Organization
		collaborator.Status.Repository = resolved.Repository.Spec.Name
	}
	if access != nil {
		collaborator.Status.Permission = githubv1alpha1.RepositoryPermission(access.Permission)
		collaborator.Status.InvitationPending = access.InvitationPending
		collaborator.Status.InvitationID = access.InvitationID
	}

	meta.SetStatusCondition(&collaborator.Status.Conditions, metav1.Condition{
		Type:               conditionTypeReady,
		Status:             status,
		ObservedGeneration: collaborator.Generation,
		Reason:             reason,
		Message:            message,
	})

	if apiequality.Semantic.DeepEqual(previousStatus, &collaborator.Status) {
		return nil
	}
	return r.Status().Update(ctx, collaborator)
}

// SetupWithManager sets up the controller with the Manager.
func (r *GitHubRepositoryCollaboratorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&githubv1alpha1.GitHubRepositoryCollaborator{}).
		Named("githubrepositorycollaborator").
		Complete(r)
}
