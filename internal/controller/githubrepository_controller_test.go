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
	repositories map[string]*githubclient.Repository
	createCalls  int
	updateCalls  int
	deleteCalls  int
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
	return &copy, nil
}

func (f *fakeRepositoryClient) CreateRepository(
	_ context.Context,
	organization string,
	name string,
	private bool,
) (*githubclient.Repository, error) {
	f.createCalls++

	visibility := "public"
	if private {
		visibility = "private"
	}

	repository := &githubclient.Repository{
		ID:         int64(f.createCalls),
		HTMLURL:    fmt.Sprintf("https://github.com/%s/%s", organization, name),
		Visibility: visibility,
	}
	f.repositories[organization+"/"+name] = repository

	copy := *repository
	return &copy, nil
}

func (f *fakeRepositoryClient) UpdateRepositoryVisibility(
	_ context.Context,
	organization string,
	name string,
	visibility string,
) (*githubclient.Repository, error) {
	key := organization + "/" + name
	repository, ok := f.repositories[key]
	if !ok {
		return nil, githubclient.ErrNotFound
	}

	f.updateCalls++
	repository.Visibility = visibility

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
			providerName = "default"
			secretName   = "github-credentials"
		)

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: "default",
				},
				Data: map[string][]byte{"token": []byte("test-token")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			provider := &githubv1alpha1.GitHubProviderConfig{
				ObjectMeta: metav1.ObjectMeta{Name: providerName},
				Spec: githubv1alpha1.GitHubProviderConfigSpec{
					Organization: "k8sready",
					APIURL:       githubv1alpha1.DefaultGitHubAPIURL,
					Credentials: githubv1alpha1.GitHubProviderCredentials{
						SecretRef: githubv1alpha1.NamespacedSecretKeyReference{
							Namespace: "default",
							Name:      secretName,
							Key:       "token",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			resource := &githubv1alpha1.GitHubRepository{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
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
				Namespace: "default",
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
			Expect(resource.Status.Organization).To(Equal("k8sready"))
			Expect(resource.Status.RepositoryID).To(Equal(int64(1)))
			Expect(resource.Status.URL).To(Equal("https://github.com/k8sready/test-resource"))
			Expect(resource.Status.Visibility).To(Equal(githubv1alpha1.RepositoryVisibilityPrivate))
			Expect(resource.Status.ObservedGeneration).To(Equal(resource.Generation))

			readyCondition := meta.FindStatusCondition(resource.Status.Conditions, "Ready")
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
			Expect(fakeGitHubClient.repositories["k8sready/test-resource"].Visibility).To(Equal("private"))

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

		It("should preserve visibility when adopting an existing repository without visibility", func() {
			fakeGitHubClient := newFakeRepositoryClient()
			fakeGitHubClient.repositories["k8sready/test-resource"] = &githubclient.Repository{
				ID:         42,
				HTMLURL:    "https://github.com/k8sready/test-resource",
				Visibility: "public",
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
			Expect(fakeGitHubClient.repositories["k8sready/test-resource"].Visibility).To(Equal("public"))

			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(resource.Status.RepositoryID).To(Equal(int64(42)))
			Expect(resource.Status.Visibility).To(Equal(githubv1alpha1.RepositoryVisibilityPublic))

			readyCondition := meta.FindStatusCondition(resource.Status.Conditions, "Ready")
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Reason).To(Equal("RepositoryAvailable"))
		})
	})
})
