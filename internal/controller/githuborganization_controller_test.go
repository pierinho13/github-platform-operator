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
	"strings"

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

type fakeOrganizationClientFactory struct {
	client *fakeOrganizationClient
}

func (f *fakeOrganizationClientFactory) NewOrganizationClient(
	_ string,
	_ string,
) (githubclient.OrganizationClient, error) {
	return f.client, nil
}

type fakeOrganizationClient struct {
	teams                   map[string]*githubclient.Team
	organizationMemberships map[string]*githubclient.Membership
	teamMemberships         map[string]*githubclient.Membership
	nextTeamID              int64
	createTeamCalls         int
	updateTeamCalls         int
	deleteTeamCalls         int
	setOrganizationCalls    int
	removeOrganizationCalls int
	setTeamMembershipCalls  int
	removeTeamMemberCalls   int
}

func newFakeOrganizationClient() *fakeOrganizationClient {
	return &fakeOrganizationClient{
		teams:                   make(map[string]*githubclient.Team),
		organizationMemberships: make(map[string]*githubclient.Membership),
		teamMemberships:         make(map[string]*githubclient.Membership),
		nextTeamID:              100,
	}
}

func (f *fakeOrganizationClient) ListTeams(
	_ context.Context,
	organization string,
) ([]githubclient.Team, error) {
	prefix := organization + "/"
	result := make([]githubclient.Team, 0)
	for key, team := range f.teams {
		if strings.HasPrefix(key, prefix) {
			result = append(result, *team)
		}
	}
	return result, nil
}

func (f *fakeOrganizationClient) GetTeam(
	_ context.Context,
	organization string,
	teamSlug string,
) (*githubclient.Team, error) {
	team, ok := f.teams[organization+"/"+teamSlug]
	if !ok {
		return nil, githubclient.ErrNotFound
	}
	copy := *team
	return &copy, nil
}

func (f *fakeOrganizationClient) CreateTeam(
	_ context.Context,
	organization string,
	input githubclient.TeamCreate,
) (*githubclient.Team, error) {
	f.createTeamCalls++
	f.nextTeamID++
	slug := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(input.Name), " ", "-"))
	team := &githubclient.Team{
		ID:          f.nextTeamID,
		Name:        input.Name,
		Slug:        slug,
		Description: input.Description,
		Privacy:     input.Privacy,
	}
	f.teams[organization+"/"+slug] = team
	copy := *team
	return &copy, nil
}

func (f *fakeOrganizationClient) UpdateTeam(
	_ context.Context,
	organization string,
	teamSlug string,
	update githubclient.TeamUpdate,
) (*githubclient.Team, error) {
	team, ok := f.teams[organization+"/"+teamSlug]
	if !ok {
		return nil, githubclient.ErrNotFound
	}
	f.updateTeamCalls++
	if update.Description != nil {
		team.Description = *update.Description
	}
	if update.Privacy != nil {
		team.Privacy = *update.Privacy
	}
	copy := *team
	return &copy, nil
}

func (f *fakeOrganizationClient) DeleteTeam(
	_ context.Context,
	organization string,
	teamSlug string,
) error {
	key := organization + "/" + teamSlug
	if _, ok := f.teams[key]; !ok {
		return githubclient.ErrNotFound
	}
	f.deleteTeamCalls++
	delete(f.teams, key)
	return nil
}

func (f *fakeOrganizationClient) GetTeamMembership(
	_ context.Context,
	organization string,
	teamSlug string,
	username string,
) (*githubclient.Membership, error) {
	membership, ok := f.teamMemberships[organization+"/"+teamSlug+"/"+username]
	if !ok {
		return nil, githubclient.ErrNotFound
	}
	copy := *membership
	return &copy, nil
}

func (f *fakeOrganizationClient) SetTeamMembership(
	_ context.Context,
	organization string,
	teamSlug string,
	username string,
	role string,
) (*githubclient.Membership, error) {
	f.setTeamMembershipCalls++
	membership := &githubclient.Membership{State: "active", Role: role}
	f.teamMemberships[organization+"/"+teamSlug+"/"+username] = membership
	copy := *membership
	return &copy, nil
}

func (f *fakeOrganizationClient) RemoveTeamMembership(
	_ context.Context,
	organization string,
	teamSlug string,
	username string,
) error {
	key := organization + "/" + teamSlug + "/" + username
	if _, ok := f.teamMemberships[key]; !ok {
		return githubclient.ErrNotFound
	}
	f.removeTeamMemberCalls++
	delete(f.teamMemberships, key)
	return nil
}

func (f *fakeOrganizationClient) GetOrganizationMembership(
	_ context.Context,
	organization string,
	username string,
) (*githubclient.Membership, error) {
	membership, ok := f.organizationMemberships[organization+"/"+username]
	if !ok {
		return nil, githubclient.ErrNotFound
	}
	copy := *membership
	return &copy, nil
}

func (f *fakeOrganizationClient) SetOrganizationMembership(
	_ context.Context,
	organization string,
	username string,
	role string,
) (*githubclient.Membership, error) {
	f.setOrganizationCalls++
	membership := &githubclient.Membership{State: "pending", Role: role}
	f.organizationMemberships[organization+"/"+username] = membership
	copy := *membership
	return &copy, nil
}

func (f *fakeOrganizationClient) RemoveOrganizationMembership(
	_ context.Context,
	organization string,
	username string,
) error {
	key := organization + "/" + username
	if _, ok := f.organizationMemberships[key]; !ok {
		return githubclient.ErrNotFound
	}
	f.removeOrganizationCalls++
	delete(f.organizationMemberships, key)
	return nil
}

var _ = Describe("GitHub organization resource controllers", func() {
	const (
		providerName = "organization-provider"
		secretName   = "organization-provider-credentials"
	)

	ctx := context.Background()

	BeforeEach(func() {
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: testDefaultName},
			Data:       map[string][]byte{testTokenKey: []byte("test-token")},
		})).To(Succeed())
		Expect(k8sClient.Create(ctx, &githubv1alpha1.GitHubProviderConfig{
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
		})).To(Succeed())
	})

	AfterEach(func() {
		cleanupOrganizationResources(ctx)
	})

	It("should create, update and delete a managed team", func() {
		resourceName := "platform-team"
		privacy := githubv1alpha1.GitHubTeamPrivacyClosed
		resource := &githubv1alpha1.GitHubTeam{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: testDefaultName},
			Spec: githubv1alpha1.GitHubTeamSpec{
				ProviderConfigRef: providerName,
				Name:              "Platform Team",
				Description:       repositoryStringPtr("Platform engineers"),
				Privacy:           &privacy,
				DeletionPolicy:    githubv1alpha1.GitHubTeamDeletionPolicyDelete,
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		fakeClient := newFakeOrganizationClient()
		reconciler := &GitHubTeamReconciler{
			Client:              k8sClient,
			APIReader:           k8sClient,
			Scheme:              k8sClient.Scheme(),
			GitHubClientFactory: &fakeOrganizationClientFactory{client: fakeClient},
		}
		request := reconcile.Request{NamespacedName: types.NamespacedName{Name: resourceName, Namespace: testDefaultName}}
		_, err := reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeClient.createTeamCalls).To(Equal(1))

		Expect(k8sClient.Get(ctx, request.NamespacedName, resource)).To(Succeed())
		Expect(resource.Status.Slug).To(Equal("platform-team"))
		condition := meta.FindStatusCondition(resource.Status.Conditions, conditionTypeReady)
		Expect(condition).NotTo(BeNil())
		Expect(condition.Reason).To(Equal("TeamCreated"))

		secretPrivacy := githubv1alpha1.GitHubTeamPrivacySecret
		resource.Spec.Privacy = &secretPrivacy
		Expect(k8sClient.Update(ctx, resource)).To(Succeed())
		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeClient.updateTeamCalls).To(Equal(1))

		Expect(k8sClient.Get(ctx, request.NamespacedName, resource)).To(Succeed())
		Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeClient.deleteTeamCalls).To(Equal(1))
	})

	It("should invite and revoke an organization member", func() {
		resourceName := "octocat-member"
		resource := &githubv1alpha1.GitHubOrganizationMember{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: testDefaultName},
			Spec: githubv1alpha1.GitHubOrganizationMemberSpec{
				ProviderConfigRef: providerName,
				Username:          bypassTestUsername,
				Role:              githubv1alpha1.GitHubOrganizationRoleMember,
				DeletionPolicy:    githubv1alpha1.GitHubMembershipDeletionPolicyRevoke,
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		fakeClient := newFakeOrganizationClient()
		reconciler := &GitHubOrganizationMemberReconciler{
			Client:              k8sClient,
			APIReader:           k8sClient,
			Scheme:              k8sClient.Scheme(),
			GitHubClientFactory: &fakeOrganizationClientFactory{client: fakeClient},
		}
		request := reconcile.Request{NamespacedName: types.NamespacedName{Name: resourceName, Namespace: testDefaultName}}
		_, err := reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeClient.setOrganizationCalls).To(Equal(1))

		Expect(k8sClient.Get(ctx, request.NamespacedName, resource)).To(Succeed())
		Expect(resource.Status.InvitationPending).To(BeTrue())
		condition := meta.FindStatusCondition(resource.Status.Conditions, conditionTypeReady)
		Expect(condition).NotTo(BeNil())
		Expect(condition.Reason).To(Equal("InvitationPending"))

		Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeClient.removeOrganizationCalls).To(Equal(1))
	})

	It("should configure and revoke a team membership", func() {
		team := &githubv1alpha1.GitHubTeam{
			ObjectMeta: metav1.ObjectMeta{Name: "devops-team", Namespace: testDefaultName},
			Spec: githubv1alpha1.GitHubTeamSpec{
				ProviderConfigRef: providerName,
				Name:              "DevOps",
			},
		}
		Expect(k8sClient.Create(ctx, team)).To(Succeed())

		resourceName := "devops-octocat"
		resource := &githubv1alpha1.GitHubTeamMembership{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: testDefaultName},
			Spec: githubv1alpha1.GitHubTeamMembershipSpec{
				TeamRef:        githubv1alpha1.GitHubTeamReference{Name: team.Name},
				Username:       bypassTestUsername,
				Role:           githubv1alpha1.GitHubTeamMembershipRoleMaintainer,
				DeletionPolicy: githubv1alpha1.GitHubMembershipDeletionPolicyRevoke,
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		fakeClient := newFakeOrganizationClient()
		fakeClient.teams[testOrganization+"/devops"] = &githubclient.Team{
			ID: 101, Name: "DevOps", Slug: "devops", Privacy: "closed",
		}
		reconciler := &GitHubTeamMembershipReconciler{
			Client:              k8sClient,
			APIReader:           k8sClient,
			Scheme:              k8sClient.Scheme(),
			GitHubClientFactory: &fakeOrganizationClientFactory{client: fakeClient},
		}
		request := reconcile.Request{NamespacedName: types.NamespacedName{Name: resourceName, Namespace: testDefaultName}}
		_, err := reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeClient.setTeamMembershipCalls).To(Equal(1))

		Expect(k8sClient.Get(ctx, request.NamespacedName, resource)).To(Succeed())
		Expect(resource.Status.TeamSlug).To(Equal("devops"))
		Expect(resource.Status.Role).To(Equal(githubv1alpha1.GitHubTeamMembershipRoleMaintainer))

		Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeClient.removeTeamMemberCalls).To(Equal(1))
	})
})

func cleanupOrganizationResources(ctx context.Context) {
	var memberships githubv1alpha1.GitHubTeamMembershipList
	_ = k8sClient.List(ctx, &memberships)
	for i := range memberships.Items {
		resource := &memberships.Items[i]
		if controllerutil.ContainsFinalizer(resource, githubTeamMembershipFinalizer) {
			controllerutil.RemoveFinalizer(resource, githubTeamMembershipFinalizer)
			_ = k8sClient.Update(ctx, resource)
		}
		_ = k8sClient.Delete(ctx, resource)
	}

	var members githubv1alpha1.GitHubOrganizationMemberList
	_ = k8sClient.List(ctx, &members)
	for i := range members.Items {
		resource := &members.Items[i]
		if controllerutil.ContainsFinalizer(resource, githubOrganizationMemberFinalizer) {
			controllerutil.RemoveFinalizer(resource, githubOrganizationMemberFinalizer)
			_ = k8sClient.Update(ctx, resource)
		}
		_ = k8sClient.Delete(ctx, resource)
	}

	var teams githubv1alpha1.GitHubTeamList
	_ = k8sClient.List(ctx, &teams)
	for i := range teams.Items {
		resource := &teams.Items[i]
		if controllerutil.ContainsFinalizer(resource, githubTeamFinalizer) {
			controllerutil.RemoveFinalizer(resource, githubTeamFinalizer)
			_ = k8sClient.Update(ctx, resource)
		}
		_ = k8sClient.Delete(ctx, resource)
	}

	provider := &githubv1alpha1.GitHubProviderConfig{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "organization-provider"}, provider); err == nil {
		if controllerutil.ContainsFinalizer(provider, githubProviderConfigFinalizer) {
			controllerutil.RemoveFinalizer(provider, githubProviderConfigFinalizer)
			_ = k8sClient.Update(ctx, provider)
		}
		_ = k8sClient.Delete(ctx, provider)
	} else {
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), fmt.Sprintf("unexpected provider error: %v", err))
	}

	secret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Name: "organization-provider-credentials", Namespace: testDefaultName,
	}, secret); err == nil {
		_ = k8sClient.Delete(ctx, secret)
	}
}
