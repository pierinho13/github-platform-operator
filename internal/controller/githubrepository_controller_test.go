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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	githubv1alpha1 "github.com/pierinho13/github-platform-operator/api/v1alpha1"
	githubclient "github.com/pierinho13/github-platform-operator/internal/github"
)

type fakeRepositoryClient struct {
	repositories      map[string]*githubclient.Repository
	createCalls       int
	updateCalls       int
	topicCalls        int
	vulnerabilityGets int
	vulnerabilitySets int
	archiveCalls      int
	deleteCalls       int
	lastCreateInput   githubclient.RepositoryCreate
}

type fakeRepositoryClientFactory struct {
	client       *fakeRepositoryClient
	calls        int
	lastToken    string
	lastAPIURL   string
	factoryError error
}

func repositoryVisibilityPtr(
	visibility githubv1alpha1.RepositoryVisibility,
) *githubv1alpha1.RepositoryVisibility {
	return &visibility
}

func repositoryStringPtr(value string) *string {
	return &value
}

func repositoryBoolPtr(value bool) *bool {
	return &value
}

func repositoryTopicsPtr(values ...string) *[]string {
	return &values
}

func newFakeRepositoryClient() *fakeRepositoryClient {
	return &fakeRepositoryClient{
		repositories: make(map[string]*githubclient.Repository),
	}
}

func (f *fakeRepositoryClientFactory) NewRepositoryClient(
	token string,
	apiURL string,
) (githubclient.RepositoryClient, error) {
	f.calls++
	f.lastToken = token
	f.lastAPIURL = apiURL
	if f.factoryError != nil {
		return nil, f.factoryError
	}

	return f.client, nil
}

func (f *fakeRepositoryClient) GetRepository(
	_ context.Context,
	organization string,
	name string,
) (*githubclient.Repository, error) {
	repository, ok := f.repositories[organization+"/"+name]
	if !ok {
		return nil, githubclient.ErrNotFound
	}

	copy := *repository
	copy.Topics = append([]string(nil), repository.Topics...)
	return &copy, nil
}

func (f *fakeRepositoryClient) CreateRepository(
	_ context.Context,
	organization string,
	input githubclient.RepositoryCreate,
) (*githubclient.Repository, error) {
	f.createCalls++
	f.lastCreateInput = input

	repository := &githubclient.Repository{
		ID:               int64(f.createCalls),
		HTMLURL:          fmt.Sprintf("https://github.com/%s/%s", organization, input.Name),
		Visibility:       input.Visibility,
		HasIssues:        true,
		HasProjects:      true,
		HasWiki:          true,
		HasDiscussions:   false,
		AllowMergeCommit: true,
		AllowRebaseMerge: true,
		AllowSquashMerge: true,
	}
	f.repositories[organization+"/"+input.Name] = repository

	copy := *repository
	return &copy, nil
}

func (f *fakeRepositoryClient) UpdateRepository(
	_ context.Context,
	organization string,
	name string,
	update githubclient.RepositoryUpdate,
) (*githubclient.Repository, error) {
	key := organization + "/" + name
	repository, ok := f.repositories[key]
	if !ok {
		return nil, githubclient.ErrNotFound
	}

	f.updateCalls++
	if update.Visibility != nil {
		repository.Visibility = *update.Visibility
	}
	if update.Description != nil {
		repository.Description = *update.Description
	}
	if update.Homepage != nil {
		repository.Homepage = *update.Homepage
	}
	if update.HasIssues != nil {
		repository.HasIssues = *update.HasIssues
	}
	if update.HasProjects != nil {
		repository.HasProjects = *update.HasProjects
	}
	if update.HasWiki != nil {
		repository.HasWiki = *update.HasWiki
	}
	if update.HasDiscussions != nil {
		repository.HasDiscussions = *update.HasDiscussions
	}
	if update.DeleteBranchOnMerge != nil {
		repository.DeleteBranchOnMerge = *update.DeleteBranchOnMerge
	}
	if update.IsTemplate != nil {
		repository.IsTemplate = *update.IsTemplate
	}
	if update.AllowAutoMerge != nil {
		repository.AllowAutoMerge = *update.AllowAutoMerge
	}
	if update.AllowMergeCommit != nil {
		repository.AllowMergeCommit = *update.AllowMergeCommit
	}
	if update.AllowRebaseMerge != nil {
		repository.AllowRebaseMerge = *update.AllowRebaseMerge
	}
	if update.AllowSquashMerge != nil {
		repository.AllowSquashMerge = *update.AllowSquashMerge
	}
	if update.MergeCommitTitle != nil {
		repository.MergeCommitTitle = *update.MergeCommitTitle
	}
	if update.MergeCommitMessage != nil {
		repository.MergeCommitMessage = *update.MergeCommitMessage
	}
	if update.SquashMergeCommitTitle != nil {
		repository.SquashMergeCommitTitle = *update.SquashMergeCommitTitle
	}
	if update.SquashMergeCommitMessage != nil {
		repository.SquashMergeCommitMessage = *update.SquashMergeCommitMessage
	}
	if update.Archived != nil {
		repository.Archived = *update.Archived
	}

	copy := *repository
	copy.Topics = append([]string(nil), repository.Topics...)
	return &copy, nil
}

func (f *fakeRepositoryClient) ReplaceRepositoryTopics(
	_ context.Context,
	organization string,
	name string,
	topics []string,
) ([]string, error) {
	key := organization + "/" + name
	repository, ok := f.repositories[key]
	if !ok {
		return nil, githubclient.ErrNotFound
	}

	f.topicCalls++
	repository.Topics = append([]string(nil), topics...)
	return append([]string(nil), repository.Topics...), nil
}

func (f *fakeRepositoryClient) GetRepositoryVulnerabilityAlerts(
	_ context.Context,
	organization string,
	name string,
) (bool, error) {
	f.vulnerabilityGets++
	repository, ok := f.repositories[organization+"/"+name]
	if !ok {
		return false, githubclient.ErrNotFound
	}
	if repository.VulnerabilityAlerts == nil {
		return false, nil
	}
	return *repository.VulnerabilityAlerts, nil
}

func (f *fakeRepositoryClient) SetRepositoryVulnerabilityAlerts(
	_ context.Context,
	organization string,
	name string,
	enabled bool,
) error {
	f.vulnerabilitySets++
	repository, ok := f.repositories[organization+"/"+name]
	if !ok {
		return githubclient.ErrNotFound
	}
	repository.VulnerabilityAlerts = repositoryBoolPtr(enabled)
	return nil
}

func (f *fakeRepositoryClient) ArchiveRepository(
	_ context.Context,
	organization string,
	name string,
) (*githubclient.Repository, error) {
	f.archiveCalls++
	repository, ok := f.repositories[organization+"/"+name]
	if !ok {
		return nil, githubclient.ErrNotFound
	}
	repository.Archived = true
	copy := *repository
	return &copy, nil
}

func (f *fakeRepositoryClient) DeleteRepository(
	_ context.Context,
	organization string,
	name string,
) error {
	key := organization + "/" + name
	if _, ok := f.repositories[key]; !ok {
		return githubclient.ErrNotFound
	}

	f.deleteCalls++
	delete(f.repositories, key)
	return nil
}

var _ = Describe("GitHubRepository Controller", func() {
	Context("When reconciling a resource through a provider config", func() {
		const (
			resourceName = "test-resource"
			providerName = testDefaultName
			secretName   = "github-credentials"
		)

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: testDefaultName,
		}

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testDefaultName,
				},
				Data: map[string][]byte{testTokenKey: []byte("test-token")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			provider := &githubv1alpha1.GitHubProviderConfig{
				ObjectMeta: metav1.ObjectMeta{Name: providerName},
				Spec: githubv1alpha1.GitHubProviderConfigSpec{
					Organization: testOrganization,
					APIURL:       githubv1alpha1.DefaultGitHubAPIURL,
					Credentials: githubv1alpha1.GitHubProviderCredentials{
						SecretRef: &githubv1alpha1.NamespacedSecretKeyReference{
							Namespace: testDefaultName,
							Name:      secretName,
							Key:       testTokenKey,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			resource := &githubv1alpha1.GitHubRepository{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: testDefaultName,
				},
				Spec: githubv1alpha1.GitHubRepositorySpec{
					ProviderConfigRef: providerName,
					Name:              resourceName,
					Visibility: repositoryVisibilityPtr(
						githubv1alpha1.RepositoryVisibilityPrivate,
					),
					DeletionPolicy: githubv1alpha1.RepositoryDeletionPolicyDelete,
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		})

		AfterEach(func() {
			resource := &githubv1alpha1.GitHubRepository{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				if controllerutil.ContainsFinalizer(resource, githubRepositoryFinalizer) {
					controllerutil.RemoveFinalizer(resource, githubRepositoryFinalizer)
					Expect(k8sClient.Update(ctx, resource)).To(Succeed())
				}
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			} else {
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}

			provider := &githubv1alpha1.GitHubProviderConfig{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: providerName}, provider); err == nil {
				if controllerutil.ContainsFinalizer(provider, githubProviderConfigFinalizer) {
					controllerutil.RemoveFinalizer(provider, githubProviderConfigFinalizer)
					Expect(k8sClient.Update(ctx, provider)).To(Succeed())
				}
				Expect(k8sClient.Delete(ctx, provider)).To(Succeed())
			}

			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Namespace: testDefaultName,
				Name:      secretName,
			}, secret); err == nil {
				Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
			}
		})

		It("should create, update and delete the GitHub repository with Delete policy", func() {
			fakeGitHubClient := newFakeRepositoryClient()
			fakeFactory := &fakeRepositoryClientFactory{client: fakeGitHubClient}
			controllerReconciler := &GitHubRepositoryReconciler{
				Client:              k8sClient,
				APIReader:           k8sClient,
				Scheme:              k8sClient.Scheme(),
				GitHubClientFactory: fakeFactory,
			}
			request := reconcile.Request{NamespacedName: typeNamespacedName}

			By("adding the finalizer before creating the external repository")
			_, err := controllerReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeGitHubClient.createCalls).To(Equal(0))

			resource := &githubv1alpha1.GitHubRepository{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(resource, githubRepositoryFinalizer)).To(BeTrue())

			By("creating the GitHub repository through the provider config")
			_, err = controllerReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeGitHubClient.createCalls).To(Equal(1))
			Expect(fakeFactory.lastToken).To(Equal("test-token"))
			Expect(fakeFactory.lastAPIURL).To(Equal(githubv1alpha1.DefaultGitHubAPIURL))

			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(resource.Status.ProviderConfigRef).To(Equal(providerName))
			Expect(resource.Status.Organization).To(Equal(testOrganization))
			Expect(resource.Status.RepositoryID).To(Equal(int64(1)))
			Expect(resource.Status.URL).To(Equal(testRepositoryHTMLURL))
			Expect(resource.Status.Visibility).To(Equal(githubv1alpha1.RepositoryVisibilityPrivate))
			Expect(resource.Status.ObservedGeneration).To(Equal(resource.Generation))

			readyCondition := meta.FindStatusCondition(resource.Status.Conditions, conditionTypeReady)
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCondition.Reason).To(Equal("RepositoryCreated"))

			By("changing the desired visibility")
			resource.Spec.Visibility = repositoryVisibilityPtr(
				githubv1alpha1.RepositoryVisibilityPublic,
			)
			Expect(k8sClient.Update(ctx, resource)).To(Succeed())

			_, err = controllerReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeGitHubClient.updateCalls).To(Equal(1))
			Expect(fakeGitHubClient.repositories["k8sready/test-resource"].Visibility).To(Equal("public"))

			By("deleting the external repository when the custom resource is deleted")
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			_, err = controllerReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeGitHubClient.deleteCalls).To(Equal(1))

			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, &githubv1alpha1.GitHubRepository{})
				return apierrors.IsNotFound(err)
			}).Should(BeTrue())
		})

		It("should orphan the GitHub repository by default", func() {
			fakeGitHubClient := newFakeRepositoryClient()
			fakeFactory := &fakeRepositoryClientFactory{client: fakeGitHubClient}
			controllerReconciler := &GitHubRepositoryReconciler{
				Client:              k8sClient,
				APIReader:           k8sClient,
				Scheme:              k8sClient.Scheme(),
				GitHubClientFactory: fakeFactory,
			}
			request := reconcile.Request{NamespacedName: typeNamespacedName}

			resource := &githubv1alpha1.GitHubRepository{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			resource.Spec.Visibility = nil
			resource.Spec.DeletionPolicy = githubv1alpha1.RepositoryDeletionPolicyOrphan
			Expect(k8sClient.Update(ctx, resource)).To(Succeed())

			By("adding the finalizer and creating a private remote repository")
			_, err := controllerReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			_, err = controllerReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeGitHubClient.createCalls).To(Equal(1))
			Expect(fakeGitHubClient.repositories["k8sready/test-resource"].Visibility).To(Equal(string(githubv1alpha1.RepositoryVisibilityPrivate)))

			By("removing only the custom resource")
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			_, err = controllerReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeGitHubClient.deleteCalls).To(Equal(0))
			Expect(fakeGitHubClient.repositories).To(HaveKey("k8sready/test-resource"))

			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, &githubv1alpha1.GitHubRepository{})
				return apierrors.IsNotFound(err)
			}).Should(BeTrue())
		})

		It("should manage basic repository settings and topics", func() {
			fakeGitHubClient := newFakeRepositoryClient()
			fakeGitHubClient.repositories["k8sready/test-resource"] = &githubclient.Repository{
				ID:             43,
				HTMLURL:        testRepositoryHTMLURL,
				Visibility:     string(githubv1alpha1.RepositoryVisibilityPrivate),
				Description:    "old description",
				Homepage:       "https://old.example.com",
				Topics:         []string{"legacy"},
				HasIssues:      true,
				HasProjects:    true,
				HasWiki:        true,
				HasDiscussions: false,
			}
			fakeFactory := &fakeRepositoryClientFactory{client: fakeGitHubClient}
			controllerReconciler := &GitHubRepositoryReconciler{
				Client:              k8sClient,
				APIReader:           k8sClient,
				Scheme:              k8sClient.Scheme(),
				GitHubClientFactory: fakeFactory,
			}
			request := reconcile.Request{NamespacedName: typeNamespacedName}

			resource := &githubv1alpha1.GitHubRepository{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			resource.Spec.Visibility = nil
			resource.Spec.Description = repositoryStringPtr("Platform API")
			resource.Spec.Homepage = repositoryStringPtr("https://platform.example.com")
			resource.Spec.Topics = repositoryTopicsPtr("Kubernetes", "platform")
			resource.Spec.Features = &githubv1alpha1.RepositoryFeatures{
				Issues:      repositoryBoolPtr(false),
				Projects:    repositoryBoolPtr(false),
				Wiki:        repositoryBoolPtr(false),
				Discussions: repositoryBoolPtr(true),
			}
			resource.Spec.DeletionPolicy = githubv1alpha1.RepositoryDeletionPolicyOrphan
			Expect(k8sClient.Update(ctx, resource)).To(Succeed())

			By("adding the finalizer and reconciling managed settings")
			_, err := controllerReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			_, err = controllerReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeGitHubClient.createCalls).To(Equal(0))
			Expect(fakeGitHubClient.updateCalls).To(Equal(1))
			Expect(fakeGitHubClient.topicCalls).To(Equal(1))

			remote := fakeGitHubClient.repositories["k8sready/test-resource"]
			Expect(remote.Visibility).To(Equal(string(githubv1alpha1.RepositoryVisibilityPrivate)))
			Expect(remote.Description).To(Equal("Platform API"))
			Expect(remote.Homepage).To(Equal("https://platform.example.com"))
			Expect(remote.Topics).To(ConsistOf("kubernetes", "platform"))
			Expect(remote.HasIssues).To(BeFalse())
			Expect(remote.HasProjects).To(BeFalse())
			Expect(remote.HasWiki).To(BeFalse())
			Expect(remote.HasDiscussions).To(BeTrue())

			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(resource.Status.Description).To(Equal("Platform API"))
			Expect(resource.Status.Homepage).To(Equal("https://platform.example.com"))
			Expect(resource.Status.Topics).To(ConsistOf("kubernetes", "platform"))
			Expect(resource.Status.Features.Issues).To(BeFalse())
			Expect(resource.Status.Features.Projects).To(BeFalse())
			Expect(resource.Status.Features.Wiki).To(BeFalse())
			Expect(resource.Status.Features.Discussions).To(BeTrue())

			readyCondition := meta.FindStatusCondition(resource.Status.Conditions, conditionTypeReady)
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Reason).To(Equal("RepositoryUpdated"))
		})

		It("should preserve visibility when adopting an existing repository without visibility", func() {
			fakeGitHubClient := newFakeRepositoryClient()
			fakeGitHubClient.repositories["k8sready/test-resource"] = &githubclient.Repository{
				ID:             42,
				HTMLURL:        testRepositoryHTMLURL,
				Visibility:     "public",
				Description:    "existing description",
				Homepage:       "https://existing.example.com",
				Topics:         []string{"existing", "repository"},
				HasIssues:      true,
				HasProjects:    false,
				HasWiki:        false,
				HasDiscussions: true,
			}
			fakeFactory := &fakeRepositoryClientFactory{client: fakeGitHubClient}
			controllerReconciler := &GitHubRepositoryReconciler{
				Client:              k8sClient,
				APIReader:           k8sClient,
				Scheme:              k8sClient.Scheme(),
				GitHubClientFactory: fakeFactory,
			}
			request := reconcile.Request{NamespacedName: typeNamespacedName}

			resource := &githubv1alpha1.GitHubRepository{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			resource.Spec.Visibility = nil
			resource.Spec.DeletionPolicy = githubv1alpha1.RepositoryDeletionPolicyOrphan
			Expect(k8sClient.Update(ctx, resource)).To(Succeed())

			By("adding the finalizer and adopting the existing repository")
			_, err := controllerReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			_, err = controllerReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeGitHubClient.createCalls).To(Equal(0))
			Expect(fakeGitHubClient.updateCalls).To(Equal(0))
			Expect(fakeGitHubClient.topicCalls).To(Equal(0))

			remote := fakeGitHubClient.repositories["k8sready/test-resource"]
			Expect(remote.Visibility).To(Equal("public"))
			Expect(remote.Description).To(Equal("existing description"))
			Expect(remote.Homepage).To(Equal("https://existing.example.com"))
			Expect(remote.Topics).To(ConsistOf("existing", "repository"))
			Expect(remote.HasIssues).To(BeTrue())
			Expect(remote.HasProjects).To(BeFalse())
			Expect(remote.HasWiki).To(BeFalse())
			Expect(remote.HasDiscussions).To(BeTrue())

			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(resource.Status.RepositoryID).To(Equal(int64(42)))
			Expect(resource.Status.Visibility).To(Equal(githubv1alpha1.RepositoryVisibilityPublic))
			Expect(resource.Status.Description).To(Equal("existing description"))
			Expect(resource.Status.Homepage).To(Equal("https://existing.example.com"))
			Expect(resource.Status.Topics).To(ConsistOf("existing", "repository"))

			readyCondition := meta.FindStatusCondition(resource.Status.Conditions, conditionTypeReady)
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Reason).To(Equal("RepositoryAvailable"))
		})

		It("should manage optional repository settings without changing omitted fields", func() {
			fakeGitHubClient := newFakeRepositoryClient()
			fakeGitHubClient.repositories["k8sready/test-resource"] = &githubclient.Repository{
				ID:                       44,
				HTMLURL:                  testRepositoryHTMLURL,
				Visibility:               string(githubv1alpha1.RepositoryVisibilityInternal),
				DeleteBranchOnMerge:      false,
				IsTemplate:               false,
				AllowAutoMerge:           false,
				AllowMergeCommit:         true,
				AllowRebaseMerge:         true,
				AllowSquashMerge:         true,
				MergeCommitTitle:         "MERGE_MESSAGE",
				MergeCommitMessage:       "PR_TITLE",
				SquashMergeCommitTitle:   "PR_TITLE",
				SquashMergeCommitMessage: "BLANK",
				VulnerabilityAlerts:      repositoryBoolPtr(false),
			}
			fakeFactory := &fakeRepositoryClientFactory{client: fakeGitHubClient}
			controllerReconciler := &GitHubRepositoryReconciler{
				Client:              k8sClient,
				APIReader:           k8sClient,
				Scheme:              k8sClient.Scheme(),
				GitHubClientFactory: fakeFactory,
			}
			request := reconcile.Request{NamespacedName: typeNamespacedName}

			resource := &githubv1alpha1.GitHubRepository{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			resource.Spec.Visibility = repositoryVisibilityPtr(githubv1alpha1.RepositoryVisibilityInternal)
			resource.Spec.DeleteBranchOnMerge = repositoryBoolPtr(true)
			resource.Spec.VulnerabilityAlerts = repositoryBoolPtr(true)
			resource.Spec.IsTemplate = repositoryBoolPtr(true)
			allowMergeCommit := false
			mergeTitle := githubv1alpha1.RepositoryMergeCommitTitlePRTitle
			squashTitle := githubv1alpha1.RepositorySquashMergeCommitTitleCommitOrPRTitle
			resource.Spec.MergeOptions = &githubv1alpha1.RepositoryMergeOptions{
				AllowMergeCommit:       &allowMergeCommit,
				MergeCommitTitle:       &mergeTitle,
				SquashMergeCommitTitle: &squashTitle,
			}
			resource.Spec.DeletionPolicy = githubv1alpha1.RepositoryDeletionPolicyOrphan
			Expect(k8sClient.Update(ctx, resource)).To(Succeed())

			_, err := controllerReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			_, err = controllerReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())

			remote := fakeGitHubClient.repositories["k8sready/test-resource"]
			Expect(remote.DeleteBranchOnMerge).To(BeTrue())
			Expect(remote.IsTemplate).To(BeTrue())
			Expect(remote.AllowMergeCommit).To(BeFalse())
			Expect(remote.AllowRebaseMerge).To(BeTrue())
			Expect(remote.MergeCommitTitle).To(Equal("PR_TITLE"))
			Expect(remote.SquashMergeCommitTitle).To(Equal("COMMIT_OR_PR_TITLE"))
			Expect(fakeGitHubClient.vulnerabilityGets).To(Equal(1))
			Expect(fakeGitHubClient.vulnerabilitySets).To(Equal(1))
			Expect(*remote.VulnerabilityAlerts).To(BeTrue())
		})

		It("should create from a template and archive on deletion", func() {
			fakeGitHubClient := newFakeRepositoryClient()
			fakeFactory := &fakeRepositoryClientFactory{client: fakeGitHubClient}
			controllerReconciler := &GitHubRepositoryReconciler{
				Client:              k8sClient,
				APIReader:           k8sClient,
				Scheme:              k8sClient.Scheme(),
				GitHubClientFactory: fakeFactory,
			}
			request := reconcile.Request{NamespacedName: typeNamespacedName}

			resource := &githubv1alpha1.GitHubRepository{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			resource.Spec.Template = &githubv1alpha1.RepositoryTemplate{
				Owner:              "k8sready",
				Repository:         "service-template",
				IncludeAllBranches: true,
			}
			resource.Spec.DeletionPolicy = githubv1alpha1.RepositoryDeletionPolicyArchive
			Expect(k8sClient.Update(ctx, resource)).To(Succeed())

			_, err := controllerReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			_, err = controllerReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeGitHubClient.lastCreateInput.Template).NotTo(BeNil())
			Expect(fakeGitHubClient.lastCreateInput.Template.Repository).To(Equal("service-template"))
			Expect(fakeGitHubClient.lastCreateInput.Template.IncludeAllBranches).To(BeTrue())

			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			_, err = controllerReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeGitHubClient.archiveCalls).To(Equal(1))
			Expect(fakeGitHubClient.deleteCalls).To(Equal(0))
			Expect(fakeGitHubClient.repositories["k8sready/test-resource"].Archived).To(BeTrue())
		})

		It("should stop remote reconciliation while the provider is suspended", func() {
			fakeGitHubClient := newFakeRepositoryClient()
			fakeFactory := &fakeRepositoryClientFactory{client: fakeGitHubClient}
			controllerReconciler := &GitHubRepositoryReconciler{
				Client:              k8sClient,
				APIReader:           k8sClient,
				Scheme:              k8sClient.Scheme(),
				GitHubClientFactory: fakeFactory,
			}
			request := reconcile.Request{NamespacedName: typeNamespacedName}

			_, err := controllerReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())

			provider := &githubv1alpha1.GitHubProviderConfig{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: providerName}, provider)).To(Succeed())
			provider.Spec.Suspended = true
			Expect(k8sClient.Update(ctx, provider)).To(Succeed())

			result, err := controllerReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(requeueInterval))
			Expect(fakeFactory.calls).To(Equal(0))
			Expect(fakeGitHubClient.createCalls).To(Equal(0))

			resource := &githubv1alpha1.GitHubRepository{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			condition := meta.FindStatusCondition(resource.Status.Conditions, conditionTypeReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal("ReconciliationSuspended"))
		})

	})
})
