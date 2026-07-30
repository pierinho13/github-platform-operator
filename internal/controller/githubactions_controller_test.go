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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	githubv1alpha1 "github.com/pierinho13/github-platform-operator/api/v1alpha1"
	githubclient "github.com/pierinho13/github-platform-operator/internal/github"
)

var _ = Describe("GitHub Actions Controllers", func() {
	const (
		providerName           = "actions-provider"
		providerSecretName     = "actions-provider-credentials"
		repositoryResourceName = "actions-repository"
		repositoryName         = "platform-actions"
		sourceSecretName       = "actions-values"
	)

	ctx := context.Background()

	BeforeEach(func() {
		createRepositoryAccessDependencies(
			ctx,
			providerName,
			providerSecretName,
			repositoryResourceName,
			repositoryName,
		)
		source := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: sourceSecretName, Namespace: "default"},
			Data: map[string][]byte{
				"token":  []byte("initial-token"),
				"region": []byte("eu-west-1"),
			},
		}
		Expect(k8sClient.Create(ctx, source)).To(Succeed())
	})

	AfterEach(func() {
		for _, cleanup := range []func(){
			func() {
				var list githubv1alpha1.GitHubActionsSecretList
				if err := k8sClient.List(ctx, &list, clientInDefaultNamespace()); err == nil {
					for i := range list.Items {
						item := &list.Items[i]
						if controllerutil.ContainsFinalizer(item, githubActionsSecretFinalizer) {
							controllerutil.RemoveFinalizer(item, githubActionsSecretFinalizer)
							Expect(k8sClient.Update(ctx, item)).To(Succeed())
						}
						Expect(k8sClient.Delete(ctx, item)).To(Succeed())
					}
				}
			},
			func() {
				var list githubv1alpha1.GitHubActionsVariableList
				if err := k8sClient.List(ctx, &list, clientInDefaultNamespace()); err == nil {
					for i := range list.Items {
						item := &list.Items[i]
						if controllerutil.ContainsFinalizer(item, githubActionsVariableFinalizer) {
							controllerutil.RemoveFinalizer(item, githubActionsVariableFinalizer)
							Expect(k8sClient.Update(ctx, item)).To(Succeed())
						}
						Expect(k8sClient.Delete(ctx, item)).To(Succeed())
					}
				}
			},
			func() {
				var list githubv1alpha1.GitHubEnvironmentList
				if err := k8sClient.List(ctx, &list, clientInDefaultNamespace()); err == nil {
					for i := range list.Items {
						item := &list.Items[i]
						if controllerutil.ContainsFinalizer(item, githubEnvironmentFinalizer) {
							controllerutil.RemoveFinalizer(item, githubEnvironmentFinalizer)
							Expect(k8sClient.Update(ctx, item)).To(Succeed())
						}
						Expect(k8sClient.Delete(ctx, item)).To(Succeed())
					}
				}
			},
		} {
			cleanup()
		}

		source := &corev1.Secret{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name: sourceSecretName, Namespace: "default",
		}, source); err == nil {
			Expect(k8sClient.Delete(ctx, source)).To(Succeed())
		}
		cleanupRepositoryAccessDependencies(
			ctx,
			providerName,
			providerSecretName,
			repositoryResourceName,
		)
	})

	It("should create a basic repository environment", func() {
		fakeClient := newFakeActionsClient()
		fakeClient.repositories["k8sready/"+repositoryName] = &githubclient.Repository{
			ID: 100, HTMLURL: "https://github.com/k8sready/" + repositoryName, Visibility: "private",
		}
		factory := &fakeActionsClientFactory{client: fakeClient}
		reconciler := &GitHubEnvironmentReconciler{
			Client: k8sClient, APIReader: k8sClient, Scheme: k8sClient.Scheme(),
			GitHubClientFactory: factory,
		}
		resource := &githubv1alpha1.GitHubEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "production", Namespace: "default"},
			Spec: githubv1alpha1.GitHubEnvironmentSpec{
				RepositoryRef: githubv1alpha1.GitHubRepositoryReference{Name: repositoryResourceName},
				Name:          "production",
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		request := reconcile.Request{NamespacedName: types.NamespacedName{Name: resource.Name, Namespace: resource.Namespace}}

		_, err := reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeClient.upsertEnvironmentCalls).To(Equal(1))

		Expect(k8sClient.Get(ctx, request.NamespacedName, resource)).To(Succeed())
		condition := meta.FindStatusCondition(resource.Status.Conditions, "Ready")
		Expect(condition).NotTo(BeNil())
		Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		Expect(condition.Reason).To(Equal("EnvironmentCreated"))
		Expect(resource.Status.EnvironmentID).To(Equal(int64(1)))
	})

	It("should synchronize and rotate a repository Actions secret", func() {
		fakeClient := newFakeActionsClient()
		fakeClient.repositories["k8sready/"+repositoryName] = &githubclient.Repository{
			ID: 101, HTMLURL: "https://github.com/k8sready/" + repositoryName, Visibility: "private",
		}
		factory := &fakeActionsClientFactory{client: fakeClient}
		reconciler := &GitHubActionsSecretReconciler{
			Client: k8sClient, APIReader: k8sClient, Scheme: k8sClient.Scheme(),
			GitHubClientFactory: factory,
		}
		resource := &githubv1alpha1.GitHubActionsSecret{
			ObjectMeta: metav1.ObjectMeta{Name: "docker-token", Namespace: "default"},
			Spec: githubv1alpha1.GitHubActionsSecretSpec{
				Target: githubv1alpha1.GitHubActionsTarget{
					RepositoryRef: &githubv1alpha1.GitHubRepositoryReference{Name: repositoryResourceName},
				},
				Name: "DOCKER_TOKEN",
				ValueFrom: githubv1alpha1.ActionsValueSource{
					SecretKeyRef: githubv1alpha1.LocalSecretKeyReference{Name: sourceSecretName, Key: "token"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		request := reconcile.Request{NamespacedName: types.NamespacedName{Name: resource.Name, Namespace: resource.Namespace}}

		_, err := reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeClient.upsertSecretCalls).To(Equal(1))

		source := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sourceSecretName, Namespace: "default"}, source)).To(Succeed())
		source.Data["token"] = []byte("rotated-token")
		Expect(k8sClient.Update(ctx, source)).To(Succeed())

		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeClient.upsertSecretCalls).To(Equal(2))

		Expect(k8sClient.Get(ctx, request.NamespacedName, resource)).To(Succeed())
		condition := meta.FindStatusCondition(resource.Status.Conditions, "Ready")
		Expect(condition).NotTo(BeNil())
		Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		Expect(resource.Status.SourceSecretResourceVersion).To(Equal(source.ResourceVersion))
	})

	It("should synchronize an organization variable with selected repositories", func() {
		fakeClient := newFakeActionsClient()
		fakeClient.repositories["k8sready/"+repositoryName] = &githubclient.Repository{
			ID: 777, HTMLURL: "https://github.com/k8sready/" + repositoryName, Visibility: "private",
		}
		factory := &fakeActionsClientFactory{client: fakeClient}
		reconciler := &GitHubActionsVariableReconciler{
			Client: k8sClient, APIReader: k8sClient, Scheme: k8sClient.Scheme(),
			GitHubClientFactory: factory,
		}
		resource := &githubv1alpha1.GitHubActionsVariable{
			ObjectMeta: metav1.ObjectMeta{Name: "cloud-region", Namespace: "default"},
			Spec: githubv1alpha1.GitHubActionsVariableSpec{
				Target: githubv1alpha1.GitHubActionsTarget{
					Organization: &githubv1alpha1.GitHubOrganizationActionsTarget{
						ProviderConfigRef: providerName,
						Visibility:        githubv1alpha1.OrganizationActionsVisibilitySelected,
						SelectedRepositoryRefs: []githubv1alpha1.GitHubRepositoryReference{
							{Name: repositoryResourceName},
						},
					},
				},
				Name: "CLOUD_REGION",
				ValueFrom: githubv1alpha1.ActionsValueSource{
					SecretKeyRef: githubv1alpha1.LocalSecretKeyReference{Name: sourceSecretName, Key: "region"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		request := reconcile.Request{NamespacedName: types.NamespacedName{Name: resource.Name, Namespace: resource.Namespace}}

		_, err := reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeClient.createVariableCalls).To(Equal(1))

		target := githubclient.ActionsTarget{
			Scope: githubclient.ActionsTargetScopeOrganization, Organization: "k8sready",
		}
		remote, err := fakeClient.GetActionsVariable(ctx, target, "CLOUD_REGION")
		Expect(err).NotTo(HaveOccurred())
		Expect(remote.Value).To(Equal("eu-west-1"))
		Expect(remote.Visibility).To(Equal("selected"))
		Expect(remote.SelectedRepositoryIDs).To(Equal([]int64{777}))
	})
})

func clientInDefaultNamespace() client.ListOption {
	return client.InNamespace("default")
}
