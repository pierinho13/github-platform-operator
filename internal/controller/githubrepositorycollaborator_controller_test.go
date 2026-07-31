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

var _ = Describe("GitHubRepositoryCollaborator Controller", func() {
	const (
		providerName           = "collaborator-provider"
		secretName             = "collaborator-credentials"
		repositoryResourceName = "collaborator-repository"
		repositoryName         = "platform-worker"
		collaboratorName       = "platform-worker-octocat"
	)

	ctx := context.Background()
	collaboratorKey := types.NamespacedName{Name: collaboratorName, Namespace: testDefaultName}

	BeforeEach(func() {
		createRepositoryAccessDependencies(
			ctx,
			providerName,
			secretName,
			repositoryResourceName,
			repositoryName,
		)

		collaborator := &githubv1alpha1.GitHubRepositoryCollaborator{
			ObjectMeta: metav1.ObjectMeta{Name: collaboratorName, Namespace: testDefaultName},
			Spec: githubv1alpha1.GitHubRepositoryCollaboratorSpec{
				RepositoryRef: githubv1alpha1.GitHubRepositoryReference{
					Name: repositoryResourceName,
				},
				Username:       bypassTestUsername,
				Permission:     githubv1alpha1.RepositoryPermissionPush,
				DeletionPolicy: githubv1alpha1.RepositoryAccessDeletionPolicyRevoke,
			},
		}
		Expect(k8sClient.Create(ctx, collaborator)).To(Succeed())
	})

	AfterEach(func() {
		collaborator := &githubv1alpha1.GitHubRepositoryCollaborator{}
		if err := k8sClient.Get(ctx, collaboratorKey, collaborator); err == nil {
			if controllerutil.ContainsFinalizer(collaborator, githubRepositoryCollaboratorFinalizer) {
				controllerutil.RemoveFinalizer(collaborator, githubRepositoryCollaboratorFinalizer)
				Expect(k8sClient.Update(ctx, collaborator)).To(Succeed())
			}
			Expect(k8sClient.Delete(ctx, collaborator)).To(Succeed())
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

	It("should configure, update and revoke direct collaborator access", func() {
		fakeClient := newFakeRepositoryAccessClient()
		fakeClient.repositories["k8sready/"+repositoryName] = &githubclient.Repository{
			ID: 20, HTMLURL: "https://github.com/k8sready/" + repositoryName, Visibility: string(githubv1alpha1.RepositoryVisibilityPrivate),
		}
		factory := &fakeRepositoryAccessClientFactory{client: fakeClient}
		reconciler := &GitHubRepositoryCollaboratorReconciler{
			Client: k8sClient, APIReader: k8sClient, Scheme: k8sClient.Scheme(),
			GitHubClientFactory: factory,
		}
		request := reconcile.Request{NamespacedName: collaboratorKey}

		By("adding the finalizer")
		_, err := reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())

		By("granting direct access")
		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeClient.setCollaboratorCalls).To(Equal(1))

		collaborator := &githubv1alpha1.GitHubRepositoryCollaborator{}
		Expect(k8sClient.Get(ctx, collaboratorKey, collaborator)).To(Succeed())
		Expect(collaborator.Status.Permission).To(Equal(githubv1alpha1.RepositoryPermissionPush))
		Expect(collaborator.Status.InvitationPending).To(BeFalse())
		condition := meta.FindStatusCondition(collaborator.Status.Conditions, conditionTypeReady)
		Expect(condition).NotTo(BeNil())
		Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		Expect(condition.Reason).To(Equal("AccessConfigured"))

		By("updating the direct permission")
		collaborator.Spec.Permission = githubv1alpha1.RepositoryPermissionMaintain
		Expect(k8sClient.Update(ctx, collaborator)).To(Succeed())
		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeClient.setCollaboratorCalls).To(Equal(2))

		By("revoking direct access on deletion")
		Expect(k8sClient.Get(ctx, collaboratorKey, collaborator)).To(Succeed())
		Expect(k8sClient.Delete(ctx, collaborator)).To(Succeed())
		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeClient.removeCollaboratorCalls).To(Equal(1))
	})

	It("should report and revoke a pending invitation", func() {
		fakeClient := newFakeRepositoryAccessClient()
		fakeClient.repositories["k8sready/"+repositoryName] = &githubclient.Repository{
			ID: 20, HTMLURL: "https://github.com/k8sready/" + repositoryName, Visibility: string(githubv1alpha1.RepositoryVisibilityPrivate),
		}
		fakeClient.inviteUsers[bypassTestUsername] = true
		factory := &fakeRepositoryAccessClientFactory{client: fakeClient}
		reconciler := &GitHubRepositoryCollaboratorReconciler{
			Client: k8sClient, APIReader: k8sClient, Scheme: k8sClient.Scheme(),
			GitHubClientFactory: factory,
		}
		request := reconcile.Request{NamespacedName: collaboratorKey}

		_, err := reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())

		collaborator := &githubv1alpha1.GitHubRepositoryCollaborator{}
		Expect(k8sClient.Get(ctx, collaboratorKey, collaborator)).To(Succeed())
		Expect(collaborator.Status.InvitationPending).To(BeTrue())
		Expect(collaborator.Status.InvitationID).NotTo(BeZero())
		condition := meta.FindStatusCondition(collaborator.Status.Conditions, conditionTypeReady)
		Expect(condition).NotTo(BeNil())
		Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		Expect(condition.Reason).To(Equal("InvitationPending"))

		Expect(k8sClient.Delete(ctx, collaborator)).To(Succeed())
		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeClient.removeCollaboratorCalls).To(Equal(1))
	})
})
