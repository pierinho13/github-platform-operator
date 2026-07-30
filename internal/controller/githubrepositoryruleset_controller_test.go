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
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	githubv1alpha1 "github.com/pierinho13/github-platform-operator/api/v1alpha1"
	githubclient "github.com/pierinho13/github-platform-operator/internal/github"
)

type fakeRepositoryRulesetClientFactory struct {
	client *fakeRepositoryRulesetClient
}

func (f *fakeRepositoryRulesetClientFactory) NewRepositoryRulesetClient(
	_ string,
	_ string,
) (githubclient.RepositoryRulesetClient, error) {
	return f.client, nil
}

type fakeRepositoryRulesetClient struct {
	repositories map[string]*githubclient.Repository
	rulesets     map[string]*githubclient.RepositoryRuleset
	nextID       int64

	createCalls int
	updateCalls int
	deleteCalls int
}

func newFakeRepositoryRulesetClient() *fakeRepositoryRulesetClient {
	return &fakeRepositoryRulesetClient{
		repositories: make(map[string]*githubclient.Repository),
		rulesets:     make(map[string]*githubclient.RepositoryRuleset),
		nextID:       100,
	}
}

func (f *fakeRepositoryRulesetClient) GetRepository(
	_ context.Context,
	organization string,
	name string,
) (*githubclient.Repository, error) {
	item, ok := f.repositories[organization+"/"+name]
	if !ok {
		return nil, githubclient.ErrNotFound
	}
	copy := *item
	return &copy, nil
}

func (f *fakeRepositoryRulesetClient) ListRepositoryRulesets(
	_ context.Context,
	organization string,
	repository string,
) ([]githubclient.RepositoryRulesetSummary, error) {
	prefix := organization + "/" + repository + "/"
	result := make([]githubclient.RepositoryRulesetSummary, 0)
	for key, item := range f.rulesets {
		if len(key) < len(prefix) || key[:len(prefix)] != prefix {
			continue
		}
		result = append(result, githubclient.RepositoryRulesetSummary{
			ID:          item.ID,
			Name:        item.Name,
			SourceType:  "Repository",
			Source:      organization + "/" + repository,
			Enforcement: item.Enforcement,
		})
	}
	return result, nil
}

func (f *fakeRepositoryRulesetClient) GetRepositoryRuleset(
	_ context.Context,
	organization string,
	repository string,
	rulesetID int64,
) (*githubclient.RepositoryRuleset, error) {
	item, ok := f.rulesets[rulesetTestKey(organization, repository, rulesetID)]
	if !ok {
		return nil, githubclient.ErrNotFound
	}
	return copyRepositoryRuleset(item), nil
}

func (f *fakeRepositoryRulesetClient) CreateRepositoryRuleset(
	_ context.Context,
	organization string,
	repository string,
	input githubclient.RepositoryRulesetUpsert,
) (*githubclient.RepositoryRuleset, error) {
	f.createCalls++
	f.nextID++
	item := repositoryRulesetFromInput(organization, repository, f.nextID, input)
	f.rulesets[rulesetTestKey(organization, repository, item.ID)] = item
	return copyRepositoryRuleset(item), nil
}

func (f *fakeRepositoryRulesetClient) UpdateRepositoryRuleset(
	_ context.Context,
	organization string,
	repository string,
	rulesetID int64,
	input githubclient.RepositoryRulesetUpsert,
) (*githubclient.RepositoryRuleset, error) {
	key := rulesetTestKey(organization, repository, rulesetID)
	if _, ok := f.rulesets[key]; !ok {
		return nil, githubclient.ErrNotFound
	}
	f.updateCalls++
	item := repositoryRulesetFromInput(organization, repository, rulesetID, input)
	f.rulesets[key] = item
	return copyRepositoryRuleset(item), nil
}

func (f *fakeRepositoryRulesetClient) DeleteRepositoryRuleset(
	_ context.Context,
	organization string,
	repository string,
	rulesetID int64,
) error {
	key := rulesetTestKey(organization, repository, rulesetID)
	if _, ok := f.rulesets[key]; !ok {
		return githubclient.ErrNotFound
	}
	f.deleteCalls++
	delete(f.rulesets, key)
	return nil
}

func rulesetTestKey(organization, repository string, rulesetID int64) string {
	return fmt.Sprintf("%s/%s/%d", organization, repository, rulesetID)
}

func repositoryRulesetFromInput(
	organization string,
	repository string,
	id int64,
	input githubclient.RepositoryRulesetUpsert,
) *githubclient.RepositoryRuleset {
	return &githubclient.RepositoryRuleset{
		ID:           id,
		Name:         input.Name,
		Target:       input.Target,
		SourceType:   "Repository",
		Source:       organization + "/" + repository,
		Enforcement:  input.Enforcement,
		BypassActors: copyRulesetBypassActors(input.BypassActors),
		Conditions:   copyRulesetConditions(input.Conditions),
		Rules:        copyRulesetRules(input.Rules),
		HTMLURL:      fmt.Sprintf("https://github.com/%s/%s/rules/%d", organization, repository, id),
	}
}

func copyRulesetBypassActors(input *[]githubclient.RulesetBypassActor) []githubclient.RulesetBypassActor {
	if input == nil {
		return nil
	}
	result := append([]githubclient.RulesetBypassActor(nil), (*input)...)
	for i := range result {
		result[i].ActorID = copyInt64Pointer(result[i].ActorID)
	}
	return result
}

func copyRepositoryRuleset(input *githubclient.RepositoryRuleset) *githubclient.RepositoryRuleset {
	if input == nil {
		return nil
	}
	copy := *input
	copy.BypassActors = append([]githubclient.RulesetBypassActor(nil), input.BypassActors...)
	copy.Conditions = copyRulesetConditions(input.Conditions)
	copy.Rules = copyRulesetRules(input.Rules)
	return &copy
}

func copyRulesetConditions(input *githubclient.RulesetConditions) *githubclient.RulesetConditions {
	if input == nil {
		return nil
	}
	copy := *input
	if input.RefName != nil {
		refCopy := *input.RefName
		refCopy.Include = append([]string{}, input.RefName.Include...)
		refCopy.Exclude = append([]string{}, input.RefName.Exclude...)
		copy.RefName = &refCopy
	}
	return &copy
}

func copyRulesetRules(input []githubclient.RulesetRule) []githubclient.RulesetRule {
	result := make([]githubclient.RulesetRule, len(input))
	for i := range input {
		result[i] = input[i]
		result[i].Parameters = append(json.RawMessage(nil), input[i].Parameters...)
	}
	return result
}

var _ = Describe("GitHubRepositoryRuleset Controller", func() {
	const (
		providerName           = "ruleset-provider"
		providerSecretName     = "ruleset-provider-credentials"
		repositoryResourceName = "ruleset-repository"
		repositoryName         = "platform-ruleset"
		rulesetResourceName    = "protect-main"
	)

	ctx := context.Background()
	rulesetKey := types.NamespacedName{
		Name:      rulesetResourceName,
		Namespace: testDefaultName,
	}

	BeforeEach(func() {
		createRepositoryAccessDependencies(
			ctx,
			providerName,
			providerSecretName,
			repositoryResourceName,
			repositoryName,
		)

		resource := &githubv1alpha1.GitHubRepositoryRuleset{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rulesetResourceName,
				Namespace: testDefaultName,
			},
			Spec: githubv1alpha1.GitHubRepositoryRulesetSpec{
				RepositoryRef: githubv1alpha1.GitHubRepositoryReference{Name: repositoryResourceName},
				Name:          "protect-main",
				Target:        githubv1alpha1.GitHubRulesetTargetBranch,
				Enforcement:   githubv1alpha1.GitHubRulesetEnforcementActive,
				Conditions: &githubv1alpha1.GitHubRulesetConditions{
					RefName: &githubv1alpha1.GitHubRulesetRefNameCondition{
						Include: []string{"~DEFAULT_BRANCH"},
						Exclude: []string{},
					},
				},
				Rules: []githubv1alpha1.GitHubRulesetRule{
					{Type: githubv1alpha1.GitHubRulesetRuleDeletion},
					{
						Type: githubv1alpha1.GitHubRulesetRulePullRequest,
						Parameters: &runtime.RawExtension{Raw: []byte(`{
							"required_approving_review_count": 1,
							"dismiss_stale_reviews_on_push": true,
							"require_code_owner_review": true,
							"require_last_push_approval": false,
							"required_review_thread_resolution": true,
							"allowed_merge_methods": ["squash"]
						}`)},
					},
				},
				DeletionPolicy: githubv1alpha1.GitHubRulesetDeletionPolicyDelete,
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
	})

	AfterEach(func() {
		resource := &githubv1alpha1.GitHubRepositoryRuleset{}
		if err := k8sClient.Get(ctx, rulesetKey, resource); err == nil {
			if controllerutil.ContainsFinalizer(resource, githubRepositoryRulesetFinalizer) {
				controllerutil.RemoveFinalizer(resource, githubRepositoryRulesetFinalizer)
				Expect(k8sClient.Update(ctx, resource)).To(Succeed())
			}
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		} else {
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}

		cleanupRepositoryAccessDependencies(
			ctx,
			providerName,
			providerSecretName,
			repositoryResourceName,
		)
	})

	It("should create, update and delete a complete repository ruleset", func() {
		fakeClient := newFakeRepositoryRulesetClient()
		fakeClient.repositories[testOrganization+"/"+repositoryName] = &githubclient.Repository{
			ID:         555,
			HTMLURL:    "https://github.com/k8sready/" + repositoryName,
			Visibility: string(githubv1alpha1.RepositoryVisibilityPrivate),
		}
		factory := &fakeRepositoryRulesetClientFactory{client: fakeClient}
		reconciler := &GitHubRepositoryRulesetReconciler{
			Client:              k8sClient,
			APIReader:           k8sClient,
			Scheme:              k8sClient.Scheme(),
			GitHubClientFactory: factory,
		}
		request := reconcile.Request{NamespacedName: rulesetKey}

		By("adding the finalizer")
		_, err := reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeClient.createCalls).To(Equal(0))

		By("creating the remote ruleset")
		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeClient.createCalls).To(Equal(1))

		for _, remote := range fakeClient.rulesets {
			Expect(remote.Conditions).NotTo(BeNil())
			Expect(remote.Conditions.RefName).NotTo(BeNil())
			Expect(remote.Conditions.RefName.Exclude).NotTo(BeNil())
			Expect(remote.Conditions.RefName.Exclude).To(BeEmpty())
		}

		resource := &githubv1alpha1.GitHubRepositoryRuleset{}
		Expect(k8sClient.Get(ctx, rulesetKey, resource)).To(Succeed())
		Expect(resource.Status.RulesetID).NotTo(BeZero())
		Expect(resource.Status.Repository).To(Equal(repositoryName))
		condition := meta.FindStatusCondition(resource.Status.Conditions, conditionTypeReady)
		Expect(condition).NotTo(BeNil())
		Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		Expect(condition.Reason).To(Equal("RulesetCreated"))

		By("updating the complete desired ruleset")
		resource.Spec.Enforcement = githubv1alpha1.GitHubRulesetEnforcementDisabled
		resource.Spec.Rules = append(resource.Spec.Rules, githubv1alpha1.GitHubRulesetRule{
			Type: githubv1alpha1.GitHubRulesetRuleRequiredStatusChecks,
			Parameters: &runtime.RawExtension{Raw: []byte(`{
				"required_status_checks": [{"context": "test"}],
				"strict_required_status_checks_policy": true,
				"do_not_enforce_on_create": false
			}`)},
		})
		Expect(k8sClient.Update(ctx, resource)).To(Succeed())

		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeClient.updateCalls).To(Equal(1))

		remote, err := fakeClient.GetRepositoryRuleset(
			ctx,
			testOrganization,
			repositoryName,
			resource.Status.RulesetID,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(remote.Enforcement).To(Equal("disabled"))
		Expect(remote.Rules).To(HaveLen(3))

		By("deleting the remote ruleset before removing the finalizer")
		Expect(k8sClient.Get(ctx, rulesetKey, resource)).To(Succeed())
		Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeClient.deleteCalls).To(Equal(1))

		Eventually(func() bool {
			err := k8sClient.Get(ctx, rulesetKey, &githubv1alpha1.GitHubRepositoryRuleset{})
			return apierrors.IsNotFound(err)
		}).Should(BeTrue())
	})
})
