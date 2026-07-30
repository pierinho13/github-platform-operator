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
	githubEnvironmentFinalizer = "github.k8sready.com/environment-finalizer"
	actionsRequeueInterval     = 5 * time.Minute
)

// GitHubEnvironmentReconciler reconciles GitHubEnvironment resources.
type GitHubEnvironmentReconciler struct {
	client.Client
	APIReader           client.Reader
	Scheme              *runtime.Scheme
	GitHubClientFactory githubclient.ActionsClientFactory
}

// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubenvironments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubenvironments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubenvironments/finalizers,verbs=update
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubrepositories;githubproviderconfigs;githubactionssecrets;githubactionsvariables,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get

// Reconcile creates or observes a basic GitHub deployment environment.
func (r *GitHubEnvironmentReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	var environment githubv1alpha1.GitHubEnvironment
	if err := r.Get(ctx, req.NamespacedName, &environment); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !environment.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &environment)
	}

	if !controllerutil.ContainsFinalizer(&environment, githubEnvironmentFinalizer) {
		controllerutil.AddFinalizer(&environment, githubEnvironmentFinalizer)
		if err := r.Update(ctx, &environment); err != nil {
			return ctrl.Result{}, fmt.Errorf("add GitHubEnvironment finalizer: %w", err)
		}
		return ctrl.Result{}, nil
	}

	resolved, err := resolveRepositoryActionsTarget(
		ctx,
		r.Client,
		r.APIReader,
		r.GitHubClientFactory,
		environment.Namespace,
		environment.Spec.RepositoryRef,
		true,
	)
	if err != nil {
		return r.fail(ctx, &environment, nil, "DependencyUnavailable", err)
	}

	remote, err := resolved.Client.GetEnvironment(
		ctx,
		resolved.Provider.Spec.Organization,
		resolved.Repository.Spec.Name,
		environment.Spec.Name,
	)
	reason := "EnvironmentAvailable"
	if errors.Is(err, githubclient.ErrNotFound) {
		remote, err = resolved.Client.UpsertEnvironment(
			ctx,
			resolved.Provider.Spec.Organization,
			resolved.Repository.Spec.Name,
			environment.Spec.Name,
		)
		reason = "EnvironmentCreated"
	}
	if err != nil {
		return r.fail(ctx, &environment, resolved, "ReconciliationFailed", fmt.Errorf(
			"reconcile GitHub environment %s/%s/%s: %w",
			resolved.Provider.Spec.Organization,
			resolved.Repository.Spec.Name,
			environment.Spec.Name,
			err,
		))
	}

	if err := r.setReadyCondition(
		ctx,
		&environment,
		resolved,
		remote,
		metav1.ConditionTrue,
		reason,
		fmt.Sprintf(
			"GitHub environment %s/%s/%s is available",
			resolved.Provider.Spec.Organization,
			resolved.Repository.Spec.Name,
			environment.Spec.Name,
		),
	); err != nil {
		return ctrl.Result{}, fmt.Errorf("update GitHubEnvironment status: %w", err)
	}

	return ctrl.Result{RequeueAfter: actionsRequeueInterval}, nil
}

func (r *GitHubEnvironmentReconciler) reconcileDelete(
	ctx context.Context,
	environment *githubv1alpha1.GitHubEnvironment,
) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	if !controllerutil.ContainsFinalizer(environment, githubEnvironmentFinalizer) {
		return ctrl.Result{}, nil
	}
	if environment.Spec.EffectiveDeletionPolicy() == githubv1alpha1.EnvironmentDeletionPolicyOrphan {
		logger.Info("orphaning GitHub environment")
		return ctrl.Result{}, r.removeFinalizer(ctx, environment)
	}

	dependency, err := r.findEnvironmentDependency(ctx, environment)
	if err != nil {
		return ctrl.Result{}, err
	}
	if dependency != "" {
		message := fmt.Sprintf("GitHubEnvironment is still referenced by %s", dependency)
		if err := r.setReadyCondition(
			ctx,
			environment,
			nil,
			nil,
			metav1.ConditionFalse,
			"EnvironmentInUse",
			message,
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("update GitHubEnvironment deletion status: %w", err)
		}
		return ctrl.Result{RequeueAfter: actionsRequeueInterval}, nil
	}

	resolved, err := resolveRepositoryActionsTarget(
		ctx,
		r.Client,
		r.APIReader,
		r.GitHubClientFactory,
		environment.Namespace,
		environment.Spec.RepositoryRef,
		false,
	)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("resolve environment dependencies for deletion: %w", err)
	}
	err = resolved.Client.DeleteEnvironment(
		ctx,
		resolved.Provider.Spec.Organization,
		resolved.Repository.Spec.Name,
		environment.Spec.Name,
	)
	if err != nil && !errors.Is(err, githubclient.ErrNotFound) {
		return ctrl.Result{}, fmt.Errorf("delete GitHub environment %q: %w", environment.Spec.Name, err)
	}
	return ctrl.Result{}, r.removeFinalizer(ctx, environment)
}

func (r *GitHubEnvironmentReconciler) findEnvironmentDependency(
	ctx context.Context,
	environment *githubv1alpha1.GitHubEnvironment,
) (string, error) {
	var secrets githubv1alpha1.GitHubActionsSecretList
	if err := r.List(ctx, &secrets, client.InNamespace(environment.Namespace)); err != nil {
		return "", fmt.Errorf("list GitHubActionsSecrets: %w", err)
	}
	for i := range secrets.Items {
		ref := secrets.Items[i].Spec.Target.EnvironmentRef
		if ref != nil && ref.Name == environment.Name {
			return fmt.Sprintf("GitHubActionsSecret %s/%s", environment.Namespace, secrets.Items[i].Name), nil
		}
	}

	var variables githubv1alpha1.GitHubActionsVariableList
	if err := r.List(ctx, &variables, client.InNamespace(environment.Namespace)); err != nil {
		return "", fmt.Errorf("list GitHubActionsVariables: %w", err)
	}
	for i := range variables.Items {
		ref := variables.Items[i].Spec.Target.EnvironmentRef
		if ref != nil && ref.Name == environment.Name {
			return fmt.Sprintf("GitHubActionsVariable %s/%s", environment.Namespace, variables.Items[i].Name), nil
		}
	}
	return "", nil
}

func (r *GitHubEnvironmentReconciler) removeFinalizer(
	ctx context.Context,
	environment *githubv1alpha1.GitHubEnvironment,
) error {
	controllerutil.RemoveFinalizer(environment, githubEnvironmentFinalizer)
	if err := r.Update(ctx, environment); err != nil {
		return fmt.Errorf("remove GitHubEnvironment finalizer: %w", err)
	}
	return nil
}

func (r *GitHubEnvironmentReconciler) fail(
	ctx context.Context,
	environment *githubv1alpha1.GitHubEnvironment,
	resolved *resolvedActionsTarget,
	reason string,
	reconcileErr error,
) (ctrl.Result, error) {
	if err := r.setReadyCondition(
		ctx,
		environment,
		resolved,
		nil,
		metav1.ConditionFalse,
		reason,
		reconcileErr.Error(),
	); err != nil {
		return ctrl.Result{}, fmt.Errorf("%w; update status: %v", reconcileErr, err)
	}
	return ctrl.Result{}, reconcileErr
}

func (r *GitHubEnvironmentReconciler) setReadyCondition(
	ctx context.Context,
	environment *githubv1alpha1.GitHubEnvironment,
	resolved *resolvedActionsTarget,
	remote *githubclient.Environment,
	status metav1.ConditionStatus,
	reason string,
	message string,
) error {
	previousStatus := environment.Status.DeepCopy()
	environment.Status.Environment = environment.Spec.Name
	environment.Status.ObservedGeneration = environment.Generation
	if resolved != nil {
		environment.Status.ProviderConfigRef = resolved.Provider.Name
		environment.Status.Organization = resolved.Provider.Spec.Organization
		environment.Status.Repository = resolved.Repository.Spec.Name
	}
	if remote != nil {
		environment.Status.EnvironmentID = remote.ID
	}
	meta.SetStatusCondition(&environment.Status.Conditions, metav1.Condition{
		Type:               conditionTypeReady,
		Status:             status,
		ObservedGeneration: environment.Generation,
		Reason:             reason,
		Message:            message,
	})
	if apiequality.Semantic.DeepEqual(previousStatus, &environment.Status) {
		return nil
	}
	return r.Status().Update(ctx, environment)
}

// SetupWithManager sets up the controller with the Manager.
func (r *GitHubEnvironmentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&githubv1alpha1.GitHubEnvironment{}).
		Named("githubenvironment").
		Complete(r)
}
