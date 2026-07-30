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

var _ = Describe("GitHubRepositoryTeamAccess Controller", func() {
	const (
		providerName           = "team-provider"
		secretName             = "team-credentials"
		repositoryResourceName = "team-repository"
		repositoryName         = "platform-api"
		accessName             = "platform-api-backend"
	)

	ctx := context.Background()
	accessKey := types.NamespacedName{Name: accessName, Namespace: "default"}

	BeforeEach(func() {
		createRepositoryAccessDependencies(
			ctx,
			providerName,
			secretName,
			repositoryResourceName,
			repositoryName,
		)

		access := &githubv1alpha1.GitHubRepositoryTeamAccess{
			ObjectMeta: metav1.ObjectMeta{Name: accessName, Namespace: "default"},
			Spec: githubv1alpha1.GitHubRepositoryTeamAccessSpec{
				RepositoryRef: githubv1alpha1.GitHubRepositoryReference{
					Name: repositoryResourceName,
				},
				TeamSlug:       "backend",
				Permission:     githubv1alpha1.RepositoryPermissionMaintain,
				DeletionPolicy: githubv1alpha1.RepositoryAccessDeletionPolicyRevoke,
			},
		}
		Expect(k8sClient.Create(ctx, access)).To(Succeed())
	})

	AfterEach(func() {
		access := &githubv1alpha1.GitHubRepositoryTeamAccess{}
		if err := k8sClient.Get(ctx, accessKey, access); err == nil {
			if controllerutil.ContainsFinalizer(access, githubRepositoryTeamAccessFinalizer) {
				controllerutil.RemoveFinalizer(access, githubRepositoryTeamAccessFinalizer)
				Expect(k8sClient.Update(ctx, access)).To(Succeed())
			}
			Expect(k8sClient.Delete(ctx, access)).To(Succeed())
		} else {
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}

		cleanupRepositoryAccessDependencies(
			ctx,
			providerName,
			secretName,
			repositoryResourceName,
		)
	})

	It("should configure, update and revoke team access", func() {
		fakeClient := newFakeRepositoryAccessClient()
		fakeClient.repositories["k8sready/"+repositoryName] = &githubclient.Repository{
			ID:         10,
			HTMLURL:    "https://github.com/k8sready/" + repositoryName,
			Visibility: "private",
		}
		factory := &fakeRepositoryAccessClientFactory{client: fakeClient}
		reconciler := &GitHubRepositoryTeamAccessReconciler{
			Client:              k8sClient,
			APIReader:           k8sClient,
			Scheme:              k8sClient.Scheme(),
			GitHubClientFactory: factory,
		}
		request := reconcile.Request{NamespacedName: accessKey}

		By("adding the finalizer")
		_, err := reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())

		By("granting team access")
		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeClient.setTeamCalls).To(Equal(1))

		access := &githubv1alpha1.GitHubRepositoryTeamAccess{}
		Expect(k8sClient.Get(ctx, accessKey, access)).To(Succeed())
		Expect(access.Status.Organization).To(Equal("k8sready"))
		Expect(access.Status.Repository).To(Equal(repositoryName))
		Expect(access.Status.Permission).To(Equal(githubv1alpha1.RepositoryPermissionMaintain))
		condition := meta.FindStatusCondition(access.Status.Conditions, "Ready")
		Expect(condition).NotTo(BeNil())
		Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		Expect(condition.Reason).To(Equal("AccessConfigured"))

		By("updating the team permission")
		access.Spec.Permission = githubv1alpha1.RepositoryPermissionAdmin
		Expect(k8sClient.Update(ctx, access)).To(Succeed())
		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeClient.setTeamCalls).To(Equal(2))

		By("revoking access when the custom resource is deleted")
		Expect(k8sClient.Get(ctx, accessKey, access)).To(Succeed())
		Expect(k8sClient.Delete(ctx, access)).To(Succeed())
		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeClient.removeTeamCalls).To(Equal(1))

		Eventually(func() bool {
			err := k8sClient.Get(ctx, accessKey, &githubv1alpha1.GitHubRepositoryTeamAccess{})
			return apierrors.IsNotFound(err)
		}).Should(BeTrue())
	})

	It("should orphan team access by default", func() {
		fakeClient := newFakeRepositoryAccessClient()
		fakeClient.repositories["k8sready/"+repositoryName] = &githubclient.Repository{
			ID: 10, HTMLURL: "https://github.com/k8sready/" + repositoryName, Visibility: "private",
		}
		factory := &fakeRepositoryAccessClientFactory{client: fakeClient}
		reconciler := &GitHubRepositoryTeamAccessReconciler{
			Client: k8sClient, APIReader: k8sClient, Scheme: k8sClient.Scheme(),
			GitHubClientFactory: factory,
		}
		request := reconcile.Request{NamespacedName: accessKey}

		access := &githubv1alpha1.GitHubRepositoryTeamAccess{}
		Expect(k8sClient.Get(ctx, accessKey, access)).To(Succeed())
		access.Spec.DeletionPolicy = githubv1alpha1.RepositoryAccessDeletionPolicyOrphan
		Expect(k8sClient.Update(ctx, access)).To(Succeed())

		_, err := reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, accessKey, access)).To(Succeed())
		Expect(k8sClient.Delete(ctx, access)).To(Succeed())
		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeClient.removeTeamCalls).To(Equal(0))
	})
})
