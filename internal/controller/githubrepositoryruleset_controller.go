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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"time"

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

const (
	githubRepositoryRulesetFinalizer = "github.k8sready.com/repository-ruleset-finalizer"
	repositoryRulesetSourceType      = "Repository"
	rulesetRequeueInterval           = 5 * time.Minute
)

// GitHubRepositoryRulesetReconciler reconciles repository rulesets.
type GitHubRepositoryRulesetReconciler struct {
	client.Client
	APIReader           client.Reader
	Scheme              *runtime.Scheme
	GitHubClientFactory githubclient.RepositoryRulesetClientFactory
	GitHubTokenProvider githubclient.TokenProvider
}

// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubrepositoryrulesets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubrepositoryrulesets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubrepositoryrulesets/finalizers,verbs=update
// +kubebuilder:rbac:groups=github.k8sready.com,resources=githubrepositories;githubproviderconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get

// Reconcile moves a GitHub repository ruleset toward its declared state.
func (r *GitHubRepositoryRulesetReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	var resource githubv1alpha1.GitHubRepositoryRuleset
	if err := r.Get(ctx, req.NamespacedName, &resource); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !resource.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &resource)
	}

	if !controllerutil.ContainsFinalizer(&resource, githubRepositoryRulesetFinalizer) {
		controllerutil.AddFinalizer(&resource, githubRepositoryRulesetFinalizer)
		if err := r.Update(ctx, &resource); err != nil {
			return ctrl.Result{}, fmt.Errorf("add GitHubRepositoryRuleset finalizer: %w", err)
		}
		return ctrl.Result{}, nil
	}

	resolved, err := r.resolve(ctx, &resource, true)
	if err != nil {
		return r.fail(ctx, &resource, nil, dependencyFailureReason(err), err)
	}

	resolvedSpec, err := resolveRepositoryRulesetBypassActors(ctx, resource.Spec, resolved)
	if err != nil {
		return r.fail(ctx, &resource, resolved, "BypassActorUnavailable", err)
	}

	desired, err := desiredRepositoryRuleset(resolvedSpec)
	if err != nil {
		return r.fail(ctx, &resource, resolved, "InvalidDesiredState", err)
	}

	remote, err := r.findRepositoryRuleset(ctx, &resource, resolved)
	reason := "RulesetAvailable"
	switch {
	case errors.Is(err, githubclient.ErrNotFound):
		remote, err = resolved.Client.CreateRepositoryRuleset(
			ctx,
			resolved.Provider.Spec.Organization,
			resolved.Repository.Spec.Name,
			desired,
		)
		reason = "RulesetCreated"
	case err != nil:
		return r.fail(ctx, &resource, resolved, "ReconciliationFailed", fmt.Errorf(
			"find GitHub repository ruleset %s/%s/%s: %w",
			resolved.Provider.Spec.Organization,
			resolved.Repository.Spec.Name,
			resource.Spec.Name,
			err,
		))
	case repositoryRulesetNeedsUpdate(remote, desired):
		remote, err = resolved.Client.UpdateRepositoryRuleset(
			ctx,
			resolved.Provider.Spec.Organization,
			resolved.Repository.Spec.Name,
			remote.ID,
			desired,
		)
		reason = "RulesetUpdated"
	}
	if err != nil {
		return r.fail(ctx, &resource, resolved, "ReconciliationFailed", fmt.Errorf(
			"reconcile GitHub repository ruleset %s/%s/%s: %w",
			resolved.Provider.Spec.Organization,
			resolved.Repository.Spec.Name,
			resource.Spec.Name,
			err,
		))
	}

	if err := r.setReadyCondition(
		ctx,
		&resource,
		resolved,
		remote,
		metav1.ConditionTrue,
		reason,
		fmt.Sprintf(
			"GitHub repository ruleset %s/%s/%s is synchronized",
			resolved.Provider.Spec.Organization,
			resolved.Repository.Spec.Name,
			resource.Spec.Name,
		),
	); err != nil {
		return ctrl.Result{}, fmt.Errorf("update GitHubRepositoryRuleset status: %w", err)
	}

	return ctrl.Result{RequeueAfter: rulesetRequeueInterval}, nil
}

type resolvedRepositoryRuleset struct {
	Repository *githubv1alpha1.GitHubRepository
	Provider   *githubv1alpha1.GitHubProviderConfig
	Client     githubclient.RepositoryRulesetClient
}

func resolveRepositoryRulesetBypassActors(
	ctx context.Context,
	spec githubv1alpha1.GitHubRepositoryRulesetSpec,
	resolved *resolvedRepositoryRuleset,
) (githubv1alpha1.GitHubRepositoryRulesetSpec, error) {
	if spec.BypassActors == nil {
		return spec, nil
	}

	actors := make([]githubv1alpha1.GitHubRulesetBypassActor, len(spec.BypassActors))
	for i := range spec.BypassActors {
		actor, err := resolveRepositoryRulesetBypassActor(ctx, spec.BypassActors[i], resolved)
		if err != nil {
			return githubv1alpha1.GitHubRepositoryRulesetSpec{}, fmt.Errorf(
				"resolve bypass actor at index %d: %w",
				i,
				err,
			)
		}
		actors[i] = actor
	}

	spec.BypassActors = actors
	return spec, nil
}

func resolveRepositoryRulesetBypassActor(
	ctx context.Context,
	actor githubv1alpha1.GitHubRulesetBypassActor,
	resolved *resolvedRepositoryRuleset,
) (githubv1alpha1.GitHubRulesetBypassActor, error) {
	hasActorID := actor.ActorID != nil
	hasTeamSlug := actor.TeamSlug != ""
	hasUsername := actor.Username != ""
	actor.ActorID = copyInt64Pointer(actor.ActorID)

	switch actor.ActorType {
	case githubv1alpha1.GitHubRulesetBypassActorTeam:
		if hasUsername || hasActorID == hasTeamSlug {
			return actor, errors.New(
				"team requires exactly one of actorID or teamSlug, and username must be omitted",
			)
		}
		if hasTeamSlug {
			actorID, err := resolved.Client.GetTeamIDBySlug(
				ctx,
				resolved.Provider.Spec.Organization,
				actor.TeamSlug,
			)
			if err != nil {
				return actor, fmt.Errorf(
					"get GitHub team %q in organization %q: %w",
					actor.TeamSlug,
					resolved.Provider.Spec.Organization,
					err,
				)
			}
			actor.ActorID = &actorID
		}
	case githubv1alpha1.GitHubRulesetBypassActorUser:
		if hasTeamSlug || hasActorID == hasUsername {
			return actor, errors.New(
				"user requires exactly one of actorID or username, and teamSlug must be omitted",
			)
		}
		if hasUsername {
			actorID, err := resolved.Client.GetUserIDByUsername(ctx, actor.Username)
			if err != nil {
				return actor, fmt.Errorf("get GitHub user %q: %w", actor.Username, err)
			}
			actor.ActorID = &actorID
		}
	case githubv1alpha1.GitHubRulesetBypassActorIntegration,
		githubv1alpha1.GitHubRulesetBypassActorRepositoryRole:
		if !hasActorID || hasTeamSlug || hasUsername {
			return actor, fmt.Errorf(
				"%s requires actorID and does not accept teamSlug or username",
				actor.ActorType,
			)
		}
	case githubv1alpha1.GitHubRulesetBypassActorOrganizationAdmin,
		githubv1alpha1.GitHubRulesetBypassActorDeployKey:
		if hasActorID || hasTeamSlug || hasUsername {
			return actor, fmt.Errorf(
				"%s does not accept actorID, teamSlug, or username",
				actor.ActorType,
			)
		}
	default:
		return actor, fmt.Errorf("unsupported bypass actor type %q", actor.ActorType)
	}

	return actor, nil
}

func (r *GitHubRepositoryRulesetReconciler) resolve(
	ctx context.Context,
	resource *githubv1alpha1.GitHubRepositoryRuleset,
	verifyRemote bool,
) (*resolvedRepositoryRuleset, error) {
	if r.GitHubClientFactory == nil {
		return nil, errors.New("GitHub repository ruleset client factory is not configured")
	}

	var repository githubv1alpha1.GitHubRepository
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: resource.Namespace,
		Name:      resource.Spec.RepositoryRef.Name,
	}, &repository); err != nil {
		return nil, fmt.Errorf(
			"get GitHubRepository %s/%s: %w",
			resource.Namespace,
			resource.Spec.RepositoryRef.Name,
			err,
		)
	}

	var provider githubv1alpha1.GitHubProviderConfig
	providerName := repository.Spec.EffectiveProviderConfigRef()
	if err := r.Get(ctx, types.NamespacedName{Name: providerName}, &provider); err != nil {
		return nil, fmt.Errorf("get GitHubProviderConfig %q: %w", providerName, err)
	}

	token, err := resolveProviderToken(
		ctx,
		r.Client,
		r.APIReader,
		r.GitHubTokenProvider,
		&provider,
	)
	if err != nil {
		return nil, err
	}

	apiURL := provider.Spec.APIURL
	if apiURL == "" {
		apiURL = githubv1alpha1.DefaultGitHubAPIURL
	}
	rulesetClient, err := r.GitHubClientFactory.NewRepositoryRulesetClient(token, apiURL)
	if err != nil {
		return nil, fmt.Errorf("create GitHub ruleset client for provider %q: %w", provider.Name, err)
	}

	if verifyRemote {
		if _, err := rulesetClient.GetRepository(
			ctx,
			provider.Spec.Organization,
			repository.Spec.Name,
		); err != nil {
			if errors.Is(err, githubclient.ErrNotFound) {
				return nil, fmt.Errorf(
					"GitHub repository %s/%s does not exist",
					provider.Spec.Organization,
					repository.Spec.Name,
				)
			}
			return nil, fmt.Errorf(
				"get GitHub repository %s/%s: %w",
				provider.Spec.Organization,
				repository.Spec.Name,
				err,
			)
		}
	}

	return &resolvedRepositoryRuleset{
		Repository: &repository,
		Provider:   &provider,
		Client:     rulesetClient,
	}, nil
}

func (r *GitHubRepositoryRulesetReconciler) findRepositoryRuleset(
	ctx context.Context,
	resource *githubv1alpha1.GitHubRepositoryRuleset,
	resolved *resolvedRepositoryRuleset,
) (*githubclient.RepositoryRuleset, error) {
	if resource.Status.RulesetID > 0 {
		remote, err := resolved.Client.GetRepositoryRuleset(
			ctx,
			resolved.Provider.Spec.Organization,
			resolved.Repository.Spec.Name,
			resource.Status.RulesetID,
		)
		if err == nil {
			return remote, nil
		}
		if !errors.Is(err, githubclient.ErrNotFound) {
			return nil, err
		}
	}

	summaries, err := resolved.Client.ListRepositoryRulesets(
		ctx,
		resolved.Provider.Spec.Organization,
		resolved.Repository.Spec.Name,
	)
	if err != nil {
		return nil, err
	}

	var matches []githubclient.RepositoryRulesetSummary
	for i := range summaries {
		if summaries[i].Name == resource.Spec.Name &&
			summaries[i].SourceType == repositoryRulesetSourceType {
			matches = append(matches, summaries[i])
		}
	}

	switch len(matches) {
	case 0:
		return nil, githubclient.ErrNotFound
	case 1:
		return resolved.Client.GetRepositoryRuleset(
			ctx,
			resolved.Provider.Spec.Organization,
			resolved.Repository.Spec.Name,
			matches[0].ID,
		)
	default:
		return nil, fmt.Errorf(
			"multiple repository rulesets named %q exist; rename or remove duplicates before adoption",
			resource.Spec.Name,
		)
	}
}

func desiredRepositoryRuleset(
	spec githubv1alpha1.GitHubRepositoryRulesetSpec,
) (githubclient.RepositoryRulesetUpsert, error) {
	result := githubclient.RepositoryRulesetUpsert{
		Name:        spec.Name,
		Target:      string(spec.EffectiveTarget()),
		Enforcement: string(spec.Enforcement),
		Rules:       make([]githubclient.RulesetRule, 0, len(spec.Rules)),
	}

	if spec.BypassActors != nil {
		bypassActors := make([]githubclient.RulesetBypassActor, 0, len(spec.BypassActors))
		for i := range spec.BypassActors {
			mode := spec.BypassActors[i].BypassMode
			if mode == "" {
				mode = githubv1alpha1.GitHubRulesetBypassModeAlways
			}
			bypassActors = append(bypassActors, githubclient.RulesetBypassActor{
				ActorID:    copyInt64Pointer(spec.BypassActors[i].ActorID),
				ActorType:  string(spec.BypassActors[i].ActorType),
				BypassMode: string(mode),
			})
		}
		result.BypassActors = &bypassActors
	}

	if spec.Conditions != nil {
		result.Conditions = &githubclient.RulesetConditions{}
		if spec.Conditions.RefName != nil {
			result.Conditions.RefName = &githubclient.RulesetRefNameCondition{
				Include: append([]string{}, spec.Conditions.RefName.Include...),
				Exclude: append([]string{}, spec.Conditions.RefName.Exclude...),
			}
		}
	}

	for i := range spec.Rules {
		rule := githubclient.RulesetRule{Type: string(spec.Rules[i].Type)}
		if spec.Rules[i].Parameters != nil && len(spec.Rules[i].Parameters.Raw) > 0 {
			if !json.Valid(spec.Rules[i].Parameters.Raw) {
				return githubclient.RepositoryRulesetUpsert{}, fmt.Errorf(
					"rule %q parameters contain invalid JSON",
					spec.Rules[i].Type,
				)
			}
			rule.Parameters = append([]byte(nil), spec.Rules[i].Parameters.Raw...)
		}
		result.Rules = append(result.Rules, rule)
	}

	return result, nil
}

func repositoryRulesetNeedsUpdate(
	remote *githubclient.RepositoryRuleset,
	desired githubclient.RepositoryRulesetUpsert,
) bool {
	if remote == nil {
		return true
	}
	current := githubclient.RepositoryRulesetUpsert{
		Name:        remote.Name,
		Target:      remote.Target,
		Enforcement: remote.Enforcement,
		Rules:       remote.Rules,
	}
	if desired.BypassActors != nil {
		bypassActors := append([]githubclient.RulesetBypassActor(nil), remote.BypassActors...)
		current.BypassActors = &bypassActors
	}
	if desired.Conditions != nil {
		current.Conditions = remote.Conditions
	}
	return !reflect.DeepEqual(
		normalizeRulesetUpsert(current),
		normalizeRulesetUpsert(desired),
	)
}

type normalizedRuleset struct {
	Name         string
	Target       string
	Enforcement  string
	BypassActors *[]normalizedBypassActor
	Conditions   *normalizedRulesetConditions
	Rules        []normalizedRule
}

type normalizedBypassActor struct {
	ActorID    *int64
	ActorType  string
	BypassMode string
}

type normalizedRulesetConditions struct {
	Include []string
	Exclude []string
}

type normalizedRule struct {
	Type       string
	Parameters string
}

func normalizeRulesetUpsert(input githubclient.RepositoryRulesetUpsert) normalizedRuleset {
	result := normalizedRuleset{
		Name:        input.Name,
		Target:      input.Target,
		Enforcement: input.Enforcement,
		Rules:       make([]normalizedRule, 0, len(input.Rules)),
	}

	if input.BypassActors != nil {
		actors := make([]normalizedBypassActor, 0, len(*input.BypassActors))
		for i := range *input.BypassActors {
			actor := (*input.BypassActors)[i]
			mode := actor.BypassMode
			if mode == "" {
				mode = "always"
			}
			actors = append(actors, normalizedBypassActor{
				ActorID:    copyInt64Pointer(actor.ActorID),
				ActorType:  actor.ActorType,
				BypassMode: mode,
			})
		}
		sort.Slice(actors, func(i, j int) bool {
			left := fmt.Sprintf("%s/%020d/%s", actors[i].ActorType, pointerValue(actors[i].ActorID), actors[i].BypassMode)
			right := fmt.Sprintf("%s/%020d/%s", actors[j].ActorType, pointerValue(actors[j].ActorID), actors[j].BypassMode)
			return left < right
		})
		result.BypassActors = &actors
	}

	if input.Conditions != nil && input.Conditions.RefName != nil {
		result.Conditions = &normalizedRulesetConditions{
			Include: append([]string(nil), input.Conditions.RefName.Include...),
			Exclude: append([]string(nil), input.Conditions.RefName.Exclude...),
		}
		sort.Strings(result.Conditions.Include)
		sort.Strings(result.Conditions.Exclude)
	}

	for i := range input.Rules {
		result.Rules = append(result.Rules, normalizedRule{
			Type:       input.Rules[i].Type,
			Parameters: canonicalJSON(input.Rules[i].Parameters),
		})
	}
	sort.Slice(result.Rules, func(i, j int) bool {
		if result.Rules[i].Type == result.Rules[j].Type {
			return result.Rules[i].Parameters < result.Rules[j].Parameters
		}
		return result.Rules[i].Type < result.Rules[j].Type
	})

	return result
}

func canonicalJSON(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return ""
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return string(raw)
	}
	return string(normalized)
}

func copyInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func pointerValue(value *int64) int64 {
	if value == nil {
		return -1
	}
	return *value
}

func (r *GitHubRepositoryRulesetReconciler) reconcileDelete(
	ctx context.Context,
	resource *githubv1alpha1.GitHubRepositoryRuleset,
) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	if !controllerutil.ContainsFinalizer(resource, githubRepositoryRulesetFinalizer) {
		return ctrl.Result{}, nil
	}

	if resource.Spec.EffectiveDeletionPolicy() == githubv1alpha1.GitHubRulesetDeletionPolicyOrphan {
		logger.Info("orphaning GitHub repository ruleset")
		return ctrl.Result{}, r.removeFinalizer(ctx, resource)
	}

	resolved, err := r.resolve(ctx, resource, false)
	if err != nil {
		if result, ok := githubDeferredResult(err); ok {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolve ruleset deletion dependencies: %w", err)
	}

	remote, err := r.findRepositoryRuleset(ctx, resource, resolved)
	if err != nil && !errors.Is(err, githubclient.ErrNotFound) {
		if result, ok := githubDeferredResult(err); ok {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf("find ruleset for deletion: %w", err)
	}
	if remote != nil {
		err = resolved.Client.DeleteRepositoryRuleset(
			ctx,
			resolved.Provider.Spec.Organization,
			resolved.Repository.Spec.Name,
			remote.ID,
		)
		if err != nil && !errors.Is(err, githubclient.ErrNotFound) {
			if result, ok := githubDeferredResult(err); ok {
				return result, nil
			}
			return ctrl.Result{}, fmt.Errorf("delete GitHub repository ruleset %q: %w", resource.Spec.Name, err)
		}
	}

	return ctrl.Result{}, r.removeFinalizer(ctx, resource)
}

func (r *GitHubRepositoryRulesetReconciler) removeFinalizer(
	ctx context.Context,
	resource *githubv1alpha1.GitHubRepositoryRuleset,
) error {
	controllerutil.RemoveFinalizer(resource, githubRepositoryRulesetFinalizer)
	if err := r.Update(ctx, resource); err != nil {
		return fmt.Errorf("remove GitHubRepositoryRuleset finalizer: %w", err)
	}
	return nil
}

func (r *GitHubRepositoryRulesetReconciler) fail(
	ctx context.Context,
	resource *githubv1alpha1.GitHubRepositoryRuleset,
	resolved *resolvedRepositoryRuleset,
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

func (r *GitHubRepositoryRulesetReconciler) setReadyCondition(
	ctx context.Context,
	resource *githubv1alpha1.GitHubRepositoryRuleset,
	resolved *resolvedRepositoryRuleset,
	remote *githubclient.RepositoryRuleset,
	status metav1.ConditionStatus,
	reason string,
	message string,
) error {
	previousStatus := resource.Status.DeepCopy()
	resource.Status.ObservedGeneration = resource.Generation
	resource.Status.RulesetName = resource.Spec.Name
	if resolved != nil {
		resource.Status.ProviderConfigRef = resolved.Provider.Name
		resource.Status.Organization = resolved.Provider.Spec.Organization
		resource.Status.Repository = resolved.Repository.Spec.Name
	}
	if remote != nil {
		resource.Status.RulesetID = remote.ID
		resource.Status.RulesetName = remote.Name
		resource.Status.Target = githubv1alpha1.GitHubRulesetTarget(remote.Target)
		resource.Status.Enforcement = githubv1alpha1.GitHubRulesetEnforcement(remote.Enforcement)
		resource.Status.URL = remote.HTMLURL
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
func (r *GitHubRepositoryRulesetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&githubv1alpha1.GitHubRepositoryRuleset{}).
		Named("githubrepositoryruleset").
		Complete(r)
}
