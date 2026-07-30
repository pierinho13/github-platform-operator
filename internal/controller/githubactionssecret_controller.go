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

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	githubv1alpha1 "github.com/pierinho13/github-platform-operator/api/v1alpha1"
	githubclient "github.com/pierinho13/github-platform-operator/internal/github"
)

const (
	githubActionsSecretFinalizer = "github.k8sready.com/actions-secret-finalizer"
	actionsSecretSourceField     = ".spec.valueFrom.secretKeyRef.name"
)

// GitHubActionsSecretReconciler reconciles GitHubActionsSecret resources.
type GitHubActionsSecretReconciler struct {
	client.Client
	APIReader           client.Reader
	Scheme              *runtime.Scheme
	GitHubClientFactory githubclient.ActionsClientFactory
	GitHubTokenProvider githubclient.TokenProvider
}

// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubactionssecrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubactionssecrets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubactionssecrets/finalizers,verbs=update
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubrepositories;githubenvironments;githubproviderconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile synchronizes a Kubernetes Secret value into GitHub Actions.
func (r *GitHubActionsSecretReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	var resource githubv1alpha1.GitHubActionsSecret
	if err := r.Get(ctx, req.NamespacedName, &resource); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !resource.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &resource)
	}
	if !controllerutil.ContainsFinalizer(&resource, githubActionsSecretFinalizer) {
		controllerutil.AddFinalizer(&resource, githubActionsSecretFinalizer)
		if err := r.Update(ctx, &resource); err != nil {
			return ctrl.Result{}, fmt.Errorf("add GitHubActionsSecret finalizer: %w", err)
		}
		return ctrl.Result{}, nil
	}

	resolved, err := resolveActionsTarget(
		ctx,
		r.Client,
		r.APIReader,
		r.GitHubClientFactory,
		r.GitHubTokenProvider,
		resource.Namespace,
		resource.Spec.Target,
	)
	if err != nil {
		return r.fail(ctx, &resource, nil, nil, dependencyFailureReason(err), err)
	}
	reader := r.APIReader
	if reader == nil {
		reader = r.Client
	}
	source, err := resolveActionsValueSource(ctx, reader, resource.Namespace, resource.Spec.ValueFrom)
	if err != nil {
		return r.fail(ctx, &resource, resolved, nil, "SourceUnavailable", err)
	}

	remote, err := resolved.Client.GetActionsSecret(ctx, resolved.Target, resource.Spec.Name)
	shouldSync := false
	reason := "SecretAvailable"
	switch {
	case errors.Is(err, githubclient.ErrNotFound):
		shouldSync = true
		reason = "SecretCreated"
	case err != nil:
		return r.fail(ctx, &resource, resolved, source, "ReconciliationFailed", fmt.Errorf(
			"get GitHub Actions secret %q: %w", resource.Spec.Name, err,
		))
	default:
		shouldSync = r.secretNeedsSynchronization(&resource, resolved, source, remote)
		if shouldSync {
			reason = "SecretUpdated"
		}
	}

	if shouldSync {
		publicKey, err := resolved.Client.GetActionsPublicKey(ctx, resolved.Target)
		if err != nil {
			return r.fail(ctx, &resource, resolved, source, "ReconciliationFailed", fmt.Errorf(
				"get GitHub Actions public key: %w", err,
			))
		}
		encryptedValue, err := githubclient.EncryptActionsSecret(publicKey.Key, source.Value)
		if err != nil {
			return r.fail(ctx, &resource, resolved, source, "EncryptionFailed", err)
		}
		input := githubclient.ActionsSecretUpsert{
			EncryptedValue: encryptedValue,
			KeyID:          publicKey.KeyID,
		}
		if resolved.Target.Scope == githubclient.ActionsTargetScopeOrganization {
			visibility := string(resolved.Visibility)
			input.Visibility = &visibility
			input.SelectedRepositoryIDs = append([]int64(nil), resolved.SelectedRepositoryIDs...)
		}
		if err := resolved.Client.UpsertActionsSecret(
			ctx,
			resolved.Target,
			resource.Spec.Name,
			input,
		); err != nil {
			return r.fail(ctx, &resource, resolved, source, "ReconciliationFailed", fmt.Errorf(
				"synchronize GitHub Actions secret %q: %w", resource.Spec.Name, err,
			))
		}
		remote, err = resolved.Client.GetActionsSecret(ctx, resolved.Target, resource.Spec.Name)
		if err != nil {
			return r.fail(ctx, &resource, resolved, source, "ReconciliationFailed", fmt.Errorf(
				"read synchronized GitHub Actions secret %q: %w", resource.Spec.Name, err,
			))
		}
	}

	if err := r.setReadyCondition(
		ctx,
		&resource,
		resolved,
		source,
		remote,
		metav1.ConditionTrue,
		reason,
		fmt.Sprintf("GitHub Actions secret %s is synchronized to %s", resource.Spec.Name, resolved.Target.Scope),
	); err != nil {
		return ctrl.Result{}, fmt.Errorf("update GitHubActionsSecret status: %w", err)
	}
	return ctrl.Result{RequeueAfter: actionsRequeueInterval}, nil
}

func (r *GitHubActionsSecretReconciler) secretNeedsSynchronization(
	resource *githubv1alpha1.GitHubActionsSecret,
	resolved *resolvedActionsTarget,
	source *resolvedValueSource,
	remote *githubclient.ActionsSecret,
) bool {
	if resource.Status.ObservedGeneration != resource.Generation ||
		resource.Status.SourceSecretUID != source.SecretUID ||
		resource.Status.SourceSecretResourceVersion != source.ResourceVersion ||
		resource.Status.RemoteUpdatedAt != formatGitHubTime(remote.UpdatedAt) {
		return true
	}
	if resolved.Target.Scope == githubclient.ActionsTargetScopeOrganization {
		if remote.Visibility != string(resolved.Visibility) {
			return true
		}
		if resolved.Visibility == githubv1alpha1.OrganizationActionsVisibilitySelected &&
			!equalInt64Sets(remote.SelectedRepositoryIDs, resolved.SelectedRepositoryIDs) {
			return true
		}
	}
	return false
}

func (r *GitHubActionsSecretReconciler) reconcileDelete(
	ctx context.Context,
	resource *githubv1alpha1.GitHubActionsSecret,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(resource, githubActionsSecretFinalizer) {
		return ctrl.Result{}, nil
	}
	if resource.Spec.EffectiveDeletionPolicy() == githubv1alpha1.ActionsResourceDeletionPolicyOrphan {
		return ctrl.Result{}, r.removeFinalizer(ctx, resource)
	}
	resolved, err := resolveActionsTargetForDeletion(
		ctx,
		r.Client,
		r.APIReader,
		r.GitHubClientFactory,
		r.GitHubTokenProvider,
		resource.Namespace,
		resource.Spec.Target,
	)
	if err != nil {
		if result, ok := githubDeferredResult(err); ok {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolve GitHubActionsSecret deletion target: %w", err)
	}
	err = resolved.Client.DeleteActionsSecret(ctx, resolved.Target, resource.Spec.Name)
	if err != nil && !errors.Is(err, githubclient.ErrNotFound) {
		if result, ok := githubDeferredResult(err); ok {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf("delete GitHub Actions secret %q: %w", resource.Spec.Name, err)
	}
	return ctrl.Result{}, r.removeFinalizer(ctx, resource)
}

func (r *GitHubActionsSecretReconciler) removeFinalizer(
	ctx context.Context,
	resource *githubv1alpha1.GitHubActionsSecret,
) error {
	controllerutil.RemoveFinalizer(resource, githubActionsSecretFinalizer)
	if err := r.Update(ctx, resource); err != nil {
		return fmt.Errorf("remove GitHubActionsSecret finalizer: %w", err)
	}
	return nil
}

func (r *GitHubActionsSecretReconciler) fail(
	ctx context.Context,
	resource *githubv1alpha1.GitHubActionsSecret,
	resolved *resolvedActionsTarget,
	source *resolvedValueSource,
	reason string,
	reconcileErr error,
) (ctrl.Result, error) {
	if err := r.setReadyCondition(
		ctx,
		resource,
		resolved,
		source,
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

func (r *GitHubActionsSecretReconciler) setReadyCondition(
	ctx context.Context,
	resource *githubv1alpha1.GitHubActionsSecret,
	resolved *resolvedActionsTarget,
	source *resolvedValueSource,
	remote *githubclient.ActionsSecret,
	status metav1.ConditionStatus,
	reason string,
	message string,
) error {
	previousStatus := resource.Status.DeepCopy()
	resource.Status.SecretName = resource.Spec.Name
	resource.Status.TargetScope = resource.Spec.Target.Scope()
	resource.Status.ObservedGeneration = resource.Generation
	if resolved != nil {
		resource.Status.ProviderConfigRef = resolved.Provider.Name
		resource.Status.Organization = resolved.Provider.Spec.Organization
		if resolved.Repository != nil {
			resource.Status.Repository = resolved.Repository.Spec.Name
		}
		if resolved.Environment != nil {
			resource.Status.Environment = resolved.Environment.Spec.Name
		}
	}
	if source != nil {
		resource.Status.SourceSecretUID = source.SecretUID
		resource.Status.SourceSecretResourceVersion = source.ResourceVersion
	}
	if remote != nil {
		resource.Status.RemoteUpdatedAt = formatGitHubTime(remote.UpdatedAt)
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

func formatGitHubTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func (r *GitHubActionsSecretReconciler) mapSourceSecret(
	ctx context.Context,
	object client.Object,
) []reconcile.Request {
	var resources githubv1alpha1.GitHubActionsSecretList
	if err := r.List(
		ctx,
		&resources,
		client.InNamespace(object.GetNamespace()),
		client.MatchingFields{actionsSecretSourceField: object.GetName()},
	); err != nil {
		return nil
	}
	requests := make([]reconcile.Request, 0, len(resources.Items))
	for i := range resources.Items {
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&resources.Items[i])})
	}
	return requests
}

// SetupWithManager sets up the controller and its source Secret watch.
func (r *GitHubActionsSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&githubv1alpha1.GitHubActionsSecret{},
		actionsSecretSourceField,
		func(object client.Object) []string {
			resource := object.(*githubv1alpha1.GitHubActionsSecret)
			return []string{resource.Spec.ValueFrom.SecretKeyRef.Name}
		},
	); err != nil {
		return fmt.Errorf("index GitHubActionsSecret source Secret: %w", err)
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&githubv1alpha1.GitHubActionsSecret{}).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(r.mapSourceSecret)).
		Named("githubactionssecret").
		Complete(r)
}
