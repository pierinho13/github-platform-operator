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
	githubActionsVariableFinalizer = "github.k8sready.com/actions-variable-finalizer"
	actionsVariableSourceField     = ".spec.valueFrom.secretKeyRef.name"
)

// GitHubActionsVariableReconciler reconciles GitHubActionsVariable resources.
type GitHubActionsVariableReconciler struct {
	client.Client
	APIReader           client.Reader
	Scheme              *runtime.Scheme
	GitHubClientFactory githubclient.ActionsClientFactory
}

// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubactionsvariables,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubactionsvariables/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubactionsvariables/finalizers,verbs=update
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubrepositories;githubenvironments;githubproviderconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile synchronizes a Kubernetes Secret value into a GitHub Actions variable.
func (r *GitHubActionsVariableReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	var resource githubv1alpha1.GitHubActionsVariable
	if err := r.Get(ctx, req.NamespacedName, &resource); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !resource.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &resource)
	}
	if !controllerutil.ContainsFinalizer(&resource, githubActionsVariableFinalizer) {
		controllerutil.AddFinalizer(&resource, githubActionsVariableFinalizer)
		if err := r.Update(ctx, &resource); err != nil {
			return ctrl.Result{}, fmt.Errorf("add GitHubActionsVariable finalizer: %w", err)
		}
		return ctrl.Result{}, nil
	}

	resolved, err := resolveActionsTarget(
		ctx,
		r.Client,
		r.APIReader,
		r.GitHubClientFactory,
		resource.Namespace,
		resource.Spec.Target,
	)
	if err != nil {
		return r.fail(ctx, &resource, nil, nil, "DependencyUnavailable", err)
	}
	reader := r.APIReader
	if reader == nil {
		reader = r.Client
	}
	source, err := resolveActionsValueSource(ctx, reader, resource.Namespace, resource.Spec.ValueFrom)
	if err != nil {
		return r.fail(ctx, &resource, resolved, nil, "SourceUnavailable", err)
	}

	desired := githubclient.ActionsVariableUpsert{
		Name:  resource.Spec.Name,
		Value: string(source.Value),
	}
	if resolved.Target.Scope == githubclient.ActionsTargetScopeOrganization {
		visibility := string(resolved.Visibility)
		desired.Visibility = &visibility
		desired.SelectedRepositoryIDs = append([]int64(nil), resolved.SelectedRepositoryIDs...)
	}

	remote, err := resolved.Client.GetActionsVariable(ctx, resolved.Target, resource.Spec.Name)
	reason := "VariableAvailable"
	switch {
	case errors.Is(err, githubclient.ErrNotFound):
		if err := resolved.Client.CreateActionsVariable(ctx, resolved.Target, desired); err != nil {
			return r.fail(ctx, &resource, resolved, source, "ReconciliationFailed", fmt.Errorf(
				"create GitHub Actions variable %q: %w", resource.Spec.Name, err,
			))
		}
		reason = "VariableCreated"
	case err != nil:
		return r.fail(ctx, &resource, resolved, source, "ReconciliationFailed", fmt.Errorf(
			"get GitHub Actions variable %q: %w", resource.Spec.Name, err,
		))
	case variableNeedsUpdate(remote, resolved, desired):
		if err := resolved.Client.UpdateActionsVariable(
			ctx,
			resolved.Target,
			resource.Spec.Name,
			desired,
		); err != nil {
			return r.fail(ctx, &resource, resolved, source, "ReconciliationFailed", fmt.Errorf(
				"update GitHub Actions variable %q: %w", resource.Spec.Name, err,
			))
		}
		reason = "VariableUpdated"
	}

	if reason != "VariableAvailable" {
		remote, err = resolved.Client.GetActionsVariable(ctx, resolved.Target, resource.Spec.Name)
		if err != nil {
			return r.fail(ctx, &resource, resolved, source, "ReconciliationFailed", fmt.Errorf(
				"read synchronized GitHub Actions variable %q: %w", resource.Spec.Name, err,
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
		fmt.Sprintf("GitHub Actions variable %s is synchronized to %s", resource.Spec.Name, resolved.Target.Scope),
	); err != nil {
		return ctrl.Result{}, fmt.Errorf("update GitHubActionsVariable status: %w", err)
	}
	return ctrl.Result{RequeueAfter: actionsRequeueInterval}, nil
}

func variableNeedsUpdate(
	remote *githubclient.ActionsVariable,
	resolved *resolvedActionsTarget,
	desired githubclient.ActionsVariableUpsert,
) bool {
	if remote.Value != desired.Value {
		return true
	}
	if resolved.Target.Scope == githubclient.ActionsTargetScopeOrganization {
		if desired.Visibility == nil || remote.Visibility != *desired.Visibility {
			return true
		}
		if resolved.Visibility == githubv1alpha1.OrganizationActionsVisibilitySelected &&
			!equalInt64Sets(remote.SelectedRepositoryIDs, desired.SelectedRepositoryIDs) {
			return true
		}
	}
	return false
}

func (r *GitHubActionsVariableReconciler) reconcileDelete(
	ctx context.Context,
	resource *githubv1alpha1.GitHubActionsVariable,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(resource, githubActionsVariableFinalizer) {
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
		resource.Namespace,
		resource.Spec.Target,
	)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("resolve GitHubActionsVariable deletion target: %w", err)
	}
	err = resolved.Client.DeleteActionsVariable(ctx, resolved.Target, resource.Spec.Name)
	if err != nil && !errors.Is(err, githubclient.ErrNotFound) {
		return ctrl.Result{}, fmt.Errorf("delete GitHub Actions variable %q: %w", resource.Spec.Name, err)
	}
	return ctrl.Result{}, r.removeFinalizer(ctx, resource)
}

func (r *GitHubActionsVariableReconciler) removeFinalizer(
	ctx context.Context,
	resource *githubv1alpha1.GitHubActionsVariable,
) error {
	controllerutil.RemoveFinalizer(resource, githubActionsVariableFinalizer)
	if err := r.Update(ctx, resource); err != nil {
		return fmt.Errorf("remove GitHubActionsVariable finalizer: %w", err)
	}
	return nil
}

func (r *GitHubActionsVariableReconciler) fail(
	ctx context.Context,
	resource *githubv1alpha1.GitHubActionsVariable,
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
	return ctrl.Result{}, reconcileErr
}

func (r *GitHubActionsVariableReconciler) setReadyCondition(
	ctx context.Context,
	resource *githubv1alpha1.GitHubActionsVariable,
	resolved *resolvedActionsTarget,
	source *resolvedValueSource,
	remote *githubclient.ActionsVariable,
	status metav1.ConditionStatus,
	reason string,
	message string,
) error {
	previousStatus := resource.Status.DeepCopy()
	resource.Status.VariableName = resource.Spec.Name
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

func (r *GitHubActionsVariableReconciler) mapSourceSecret(
	ctx context.Context,
	object client.Object,
) []reconcile.Request {
	var resources githubv1alpha1.GitHubActionsVariableList
	if err := r.List(
		ctx,
		&resources,
		client.InNamespace(object.GetNamespace()),
		client.MatchingFields{actionsVariableSourceField: object.GetName()},
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
func (r *GitHubActionsVariableReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&githubv1alpha1.GitHubActionsVariable{},
		actionsVariableSourceField,
		func(object client.Object) []string {
			resource := object.(*githubv1alpha1.GitHubActionsVariable)
			return []string{resource.Spec.ValueFrom.SecretKeyRef.Name}
		},
	); err != nil {
		return fmt.Errorf("index GitHubActionsVariable source Secret: %w", err)
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&githubv1alpha1.GitHubActionsVariable{}).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(r.mapSourceSecret)).
		Named("githubactionsvariable").
		Complete(r)
}
