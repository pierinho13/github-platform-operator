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
	"fmt"
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

	corev1 "k8s.io/api/core/v1"

	githubv1alpha1 "github.com/pierinho13/github-platform-operator/api/v1alpha1"
)

const (
	githubProviderConfigFinalizer = "github.k8sready.com/provider-config-finalizer"
	providerRequeueInterval       = time.Minute
)

// GitHubProviderConfigReconciler validates reusable GitHub provider configurations.
type GitHubProviderConfigReconciler struct {
	client.Client
	APIReader client.Reader
	Scheme    *runtime.Scheme
}

// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubproviderconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubproviderconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubproviderconfigs/finalizers,verbs=update
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubrepositories;githubactionssecrets;githubactionsvariables,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get

// Reconcile validates the referenced Secret and prevents deleting providers that are still in use.
func (r *GitHubProviderConfigReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	var provider githubv1alpha1.GitHubProviderConfig
	if err := r.Get(ctx, req.NamespacedName, &provider); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !provider.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &provider)
	}

	if !controllerutil.ContainsFinalizer(&provider, githubProviderConfigFinalizer) {
		controllerutil.AddFinalizer(&provider, githubProviderConfigFinalizer)
		if err := r.Update(ctx, &provider); err != nil {
			return ctrl.Result{}, fmt.Errorf("add GitHubProviderConfig finalizer: %w", err)
		}

		return ctrl.Result{}, nil
	}

	if err := r.validateCredentials(ctx, &provider); err != nil {
		if statusErr := r.setProviderReadyCondition(
			ctx,
			&provider,
			metav1.ConditionFalse,
			"CredentialsUnavailable",
			err.Error(),
		); statusErr != nil {
			return ctrl.Result{}, fmt.Errorf("%w; update provider status: %v", err, statusErr)
		}

		return ctrl.Result{RequeueAfter: providerRequeueInterval}, nil
	}

	if err := r.setProviderReadyCondition(
		ctx,
		&provider,
		metav1.ConditionTrue,
		"CredentialsAvailable",
		"GitHub credentials Secret is available",
	); err != nil {
		return ctrl.Result{}, fmt.Errorf("update GitHubProviderConfig status: %w", err)
	}

	return ctrl.Result{RequeueAfter: providerRequeueInterval}, nil
}

func (r *GitHubProviderConfigReconciler) validateCredentials(
	ctx context.Context,
	provider *githubv1alpha1.GitHubProviderConfig,
) error {
	reader := r.APIReader
	if reader == nil {
		reader = r.Client
	}

	secretRef := provider.Spec.Credentials.SecretRef
	var secret corev1.Secret
	if err := reader.Get(ctx, types.NamespacedName{
		Namespace: secretRef.Namespace,
		Name:      secretRef.Name,
	}, &secret); err != nil {
		return fmt.Errorf(
			"get credentials Secret %s/%s: %w",
			secretRef.Namespace,
			secretRef.Name,
			err,
		)
	}

	token, ok := secret.Data[secretRef.Key]
	if !ok {
		return fmt.Errorf(
			"credentials Secret %s/%s does not contain key %q",
			secretRef.Namespace,
			secretRef.Name,
			secretRef.Key,
		)
	}
	if strings.TrimSpace(string(token)) == "" {
		return fmt.Errorf(
			"credentials Secret %s/%s contains an empty key %q",
			secretRef.Namespace,
			secretRef.Name,
			secretRef.Key,
		)
	}

	return nil
}

func (r *GitHubProviderConfigReconciler) reconcileDelete(
	ctx context.Context,
	provider *githubv1alpha1.GitHubProviderConfig,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(provider, githubProviderConfigFinalizer) {
		return ctrl.Result{}, nil
	}

	var repositories githubv1alpha1.GitHubRepositoryList
	if err := r.List(ctx, &repositories); err != nil {
		return ctrl.Result{}, fmt.Errorf("list GitHubRepositories: %w", err)
	}

	for i := range repositories.Items {
		repository := &repositories.Items[i]
		if repository.Spec.EffectiveProviderConfigRef() == provider.Name {
			message := fmt.Sprintf(
				"GitHubProviderConfig is still referenced by GitHubRepository %s/%s",
				repository.Namespace,
				repository.Name,
			)
			if err := r.setProviderReadyCondition(
				ctx,
				provider,
				metav1.ConditionFalse,
				"ProviderInUse",
				message,
			); err != nil {
				return ctrl.Result{}, fmt.Errorf("update provider deletion status: %w", err)
			}

			return ctrl.Result{RequeueAfter: providerRequeueInterval}, nil
		}
	}

	var actionsSecrets githubv1alpha1.GitHubActionsSecretList
	if err := r.List(ctx, &actionsSecrets); err != nil {
		return ctrl.Result{}, fmt.Errorf("list GitHubActionsSecrets: %w", err)
	}
	for i := range actionsSecrets.Items {
		resource := &actionsSecrets.Items[i]
		if resource.Spec.Target.Organization != nil &&
			resource.Spec.Target.Organization.EffectiveProviderConfigRef() == provider.Name {
			return r.providerInUse(
				ctx,
				provider,
				fmt.Sprintf("GitHubActionsSecret %s/%s", resource.Namespace, resource.Name),
			)
		}
	}

	var actionsVariables githubv1alpha1.GitHubActionsVariableList
	if err := r.List(ctx, &actionsVariables); err != nil {
		return ctrl.Result{}, fmt.Errorf("list GitHubActionsVariables: %w", err)
	}
	for i := range actionsVariables.Items {
		resource := &actionsVariables.Items[i]
		if resource.Spec.Target.Organization != nil &&
			resource.Spec.Target.Organization.EffectiveProviderConfigRef() == provider.Name {
			return r.providerInUse(
				ctx,
				provider,
				fmt.Sprintf("GitHubActionsVariable %s/%s", resource.Namespace, resource.Name),
			)
		}
	}

	controllerutil.RemoveFinalizer(provider, githubProviderConfigFinalizer)
	if err := r.Update(ctx, provider); err != nil {
		return ctrl.Result{}, fmt.Errorf("remove GitHubProviderConfig finalizer: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *GitHubProviderConfigReconciler) providerInUse(
	ctx context.Context,
	provider *githubv1alpha1.GitHubProviderConfig,
	dependency string,
) (ctrl.Result, error) {
	message := fmt.Sprintf("GitHubProviderConfig is still referenced by %s", dependency)
	if err := r.setProviderReadyCondition(
		ctx,
		provider,
		metav1.ConditionFalse,
		"ProviderInUse",
		message,
	); err != nil {
		return ctrl.Result{}, fmt.Errorf("update provider deletion status: %w", err)
	}
	return ctrl.Result{RequeueAfter: providerRequeueInterval}, nil
}

func (r *GitHubProviderConfigReconciler) setProviderReadyCondition(
	ctx context.Context,
	provider *githubv1alpha1.GitHubProviderConfig,
	conditionStatus metav1.ConditionStatus,
	reason string,
	message string,
) error {
	previousStatus := provider.Status.DeepCopy()
	provider.Status.ObservedGeneration = provider.Generation

	meta.SetStatusCondition(&provider.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             conditionStatus,
		ObservedGeneration: provider.Generation,
		Reason:             reason,
		Message:            message,
	})

	if apiequality.Semantic.DeepEqual(previousStatus, &provider.Status) {
		return nil
	}

	return r.Status().Update(ctx, provider)
}

// SetupWithManager sets up the controller with the Manager.
func (r *GitHubProviderConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&githubv1alpha1.GitHubProviderConfig{}).
		Named("githubproviderconfig").
		Complete(r)
}
