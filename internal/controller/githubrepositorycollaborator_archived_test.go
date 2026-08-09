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
	"net/http"

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

var _ = Describe("GitHubRepositoryCollaborator archived repository handling", func() {
	const (
		providerName           = "archived-collaborator-provider"
		secretName             = "archived-collaborator-credentials"
		repositoryResourceName = "archived-collaborator-repository"
		repositoryName         = "archived-platform-worker"
		collaboratorName       = "archived-platform-worker-octocat"
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
				RepositoryRef:  githubv1alpha1.GitHubRepositoryReference{Name: repositoryResourceName},
				Username:       "octocat",
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

	It("should pause instead of writing collaborator access when the repository is archived", func() {
		fakeClient := newFakeRepositoryAccessClient()
		fakeClient.repositories["k8sready/"+repositoryName] = &githubclient.Repository{
			ID:         30,
			HTMLURL:    "https://github.com/k8sready/" + repositoryName,
			Visibility: string(githubv1alpha1.RepositoryVisibilityPrivate),
			Archived:   true,
		}
		factory := &fakeRepositoryAccessClientFactory{client: fakeClient}
		reconciler := &GitHubRepositoryCollaboratorReconciler{
			Client:              k8sClient,
			APIReader:           k8sClient,
			Scheme:              k8sClient.Scheme(),
			GitHubClientFactory: factory,
		}
		request := reconcile.Request{NamespacedName: collaboratorKey}

		By("adding the finalizer")
		_, err := reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())

		By("observing the archived repository without attempting a write")
		result, err := reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(repositoryAccessRequeueInterval))
		Expect(fakeClient.setCollaboratorCalls).To(Equal(0))
		Expect(fakeClient.updateInvitationCalls).To(Equal(0))

		collaborator := &githubv1alpha1.GitHubRepositoryCollaborator{}
		Expect(k8sClient.Get(ctx, collaboratorKey, collaborator)).To(Succeed())
		condition := meta.FindStatusCondition(collaborator.Status.Conditions, conditionTypeReady)
		Expect(condition).NotTo(BeNil())
		Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		Expect(condition.Reason).To(Equal("RepositoryArchived"))
		Expect(condition.Message).To(ContainSubstring("archived and read-only"))
	})

	It("should classify only archived-repository forbidden responses as archived", func() {
		Expect(isArchivedRepositoryError(&githubclient.APIError{
			StatusCode: http.StatusForbidden,
			Body:       `{"message":"Repository was archived so is read-only."}`,
		})).To(BeTrue())

		Expect(isArchivedRepositoryError(&githubclient.APIError{
			StatusCode: http.StatusForbidden,
			Body:       `{"message":"Resource not accessible by personal access token"}`,
		})).To(BeFalse())

		Expect(isArchivedRepositoryError(&githubclient.APIError{
			StatusCode: http.StatusInternalServerError,
			Body:       `{"message":"Repository was archived so is read-only."}`,
		})).To(BeFalse())
	})
})
