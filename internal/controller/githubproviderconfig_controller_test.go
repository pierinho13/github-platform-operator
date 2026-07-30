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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	githubv1alpha1 "github.com/pierinho13/github-platform-operator/api/v1alpha1"
)

var _ = Describe("GitHubProviderConfig Controller", func() {
	const (
		providerName = "provider-test"
		secretName   = "provider-test-credentials"
	)

	ctx := context.Background()
	providerKey := types.NamespacedName{Name: providerName}

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
	})

	AfterEach(func() {
		provider := &githubv1alpha1.GitHubProviderConfig{}
		if err := k8sClient.Get(ctx, providerKey, provider); err == nil {
			if controllerutil.ContainsFinalizer(provider, githubProviderConfigFinalizer) {
				controllerutil.RemoveFinalizer(provider, githubProviderConfigFinalizer)
				Expect(k8sClient.Update(ctx, provider)).To(Succeed())
			}
			Expect(k8sClient.Delete(ctx, provider)).To(Succeed())
		} else {
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}

		secret := &corev1.Secret{}
		secretKey := types.NamespacedName{Namespace: "default", Name: secretName}
		if err := k8sClient.Get(ctx, secretKey, secret); err == nil {
			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
		}
	})

	It("should validate the credentials Secret", func() {
		reconciler := &GitHubProviderConfigReconciler{
			Client:    k8sClient,
			APIReader: k8sClient,
			Scheme:    k8sClient.Scheme(),
		}
		request := reconcile.Request{NamespacedName: providerKey}

		By("adding the provider finalizer")
		_, err := reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())

		By("marking the provider ready when the Secret is available")
		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())

		provider := &githubv1alpha1.GitHubProviderConfig{}
		Expect(k8sClient.Get(ctx, providerKey, provider)).To(Succeed())
		condition := meta.FindStatusCondition(provider.Status.Conditions, "Ready")
		Expect(condition).NotTo(BeNil())
		Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		Expect(condition.Reason).To(Equal("CredentialsAvailable"))
	})
})
