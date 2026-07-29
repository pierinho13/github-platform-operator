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

func newFakeRepositoryClient() *fakeRepositoryClient {
	return &fakeRepositoryClient{
		repositories: make(map[string]*githubclient.Repository),
	}
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
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		BeforeEach(func() {
			resource := &githubv1alpha1.GitHubRepository{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: githubv1alpha1.GitHubRepositorySpec{
					Organization: "k8sready",
					Name:         resourceName,
					Visibility:   githubv1alpha1.RepositoryVisibilityPrivate,
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		})

		AfterEach(func() {
			resource := &githubv1alpha1.GitHubRepository{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if apierrors.IsNotFound(err) {
				return
			}
			Expect(err).NotTo(HaveOccurred())

			if controllerutil.ContainsFinalizer(resource, githubRepositoryFinalizer) {
				controllerutil.RemoveFinalizer(resource, githubRepositoryFinalizer)
				Expect(k8sClient.Update(ctx, resource)).To(Succeed())
			}

			err = k8sClient.Delete(ctx, resource)
			Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("should create, update and delete the GitHub repository", func() {
			fakeGitHubClient := newFakeRepositoryClient()
			controllerReconciler := &GitHubRepositoryReconciler{
				Client:       k8sClient,
				Scheme:       k8sClient.Scheme(),
				GitHubClient: fakeGitHubClient,
			}
			request := reconcile.Request{NamespacedName: typeNamespacedName}

			By("adding the finalizer before creating the external repository")
			_, err := controllerReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeGitHubClient.createCalls).To(Equal(0))

			resource := &githubv1alpha1.GitHubRepository{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(resource, githubRepositoryFinalizer)).To(BeTrue())

			By("creating the GitHub repository and updating status")
			_, err = controllerReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeGitHubClient.createCalls).To(Equal(1))

			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(resource.Status.RepositoryID).To(Equal(int64(1)))
			Expect(resource.Status.URL).To(Equal("https://github.com/k8sready/test-resource"))
			Expect(resource.Status.Visibility).To(Equal(githubv1alpha1.RepositoryVisibilityPrivate))
			Expect(resource.Status.ObservedGeneration).To(Equal(resource.Generation))

			readyCondition := meta.FindStatusCondition(resource.Status.Conditions, "Ready")
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCondition.Reason).To(Equal("RepositoryCreated"))

			By("reconciling again without creating a duplicate repository")
			_, err = controllerReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeGitHubClient.createCalls).To(Equal(1))
			Expect(fakeGitHubClient.updateCalls).To(Equal(0))

			By("changing the desired visibility")
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			resource.Spec.Visibility = githubv1alpha1.RepositoryVisibilityPublic
			Expect(k8sClient.Update(ctx, resource)).To(Succeed())

			_, err = controllerReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeGitHubClient.updateCalls).To(Equal(1))
			Expect(fakeGitHubClient.repositories["k8sready/test-resource"].Visibility).To(Equal("public"))

			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(resource.Status.Visibility).To(Equal(githubv1alpha1.RepositoryVisibilityPublic))
			readyCondition = meta.FindStatusCondition(resource.Status.Conditions, "Ready")
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Reason).To(Equal("RepositoryUpdated"))

			By("deleting the external repository when the custom resource is deleted")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			_, err = controllerReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeGitHubClient.deleteCalls).To(Equal(1))
			Expect(fakeGitHubClient.repositories).NotTo(HaveKey("k8sready/test-resource"))

			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, &githubv1alpha1.GitHubRepository{})
				return apierrors.IsNotFound(err)
			}).Should(BeTrue())
		})
	})
})
