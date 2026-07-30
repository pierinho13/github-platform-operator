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

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	githubv1alpha1 "github.com/pierinho13/github-platform-operator/api/v1alpha1"
	githubclient "github.com/pierinho13/github-platform-operator/internal/github"
)

type fakeRepositoryAccessClient struct {
	repositories  map[string]*githubclient.Repository
	teamAccess    map[string]string
	collaborators map[string]*githubclient.CollaboratorAccess
	inviteUsers   map[string]bool

	setTeamCalls            int
	removeTeamCalls         int
	setCollaboratorCalls    int
	updateInvitationCalls   int
	removeCollaboratorCalls int
}

func newFakeRepositoryAccessClient() *fakeRepositoryAccessClient {
	return &fakeRepositoryAccessClient{
		repositories:  make(map[string]*githubclient.Repository),
		teamAccess:    make(map[string]string),
		collaborators: make(map[string]*githubclient.CollaboratorAccess),
		inviteUsers:   make(map[string]bool),
	}
}

type fakeRepositoryAccessClientFactory struct {
	client     *fakeRepositoryAccessClient
	lastToken  string
	lastAPIURL string
}

func (f *fakeRepositoryAccessClientFactory) NewRepositoryAccessClient(
	token string,
	apiURL string,
) (githubclient.RepositoryAccessClient, error) {
	f.lastToken = token
	f.lastAPIURL = apiURL
	return f.client, nil
}

func (f *fakeRepositoryAccessClient) GetRepository(
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

func teamAccessKey(organization, teamSlug, repositoryOwner, repositoryName string) string {
	return fmt.Sprintf("%s/%s/%s/%s", organization, teamSlug, repositoryOwner, repositoryName)
}

func collaboratorAccessKey(repositoryOwner, repositoryName, username string) string {
	return fmt.Sprintf("%s/%s/%s", repositoryOwner, repositoryName, username)
}

func (f *fakeRepositoryAccessClient) GetTeamRepositoryPermission(
	_ context.Context,
	organization string,
	teamSlug string,
	repositoryOwner string,
	repositoryName string,
) (string, error) {
	permission, ok := f.teamAccess[teamAccessKey(
		organization,
		teamSlug,
		repositoryOwner,
		repositoryName,
	)]
	if !ok {
		return "", githubclient.ErrNotFound
	}
	return permission, nil
}

func (f *fakeRepositoryAccessClient) SetTeamRepositoryPermission(
	_ context.Context,
	organization string,
	teamSlug string,
	repositoryOwner string,
	repositoryName string,
	permission string,
) error {
	f.setTeamCalls++
	f.teamAccess[teamAccessKey(organization, teamSlug, repositoryOwner, repositoryName)] = permission
	return nil
}

func (f *fakeRepositoryAccessClient) RemoveTeamRepository(
	_ context.Context,
	organization string,
	teamSlug string,
	repositoryOwner string,
	repositoryName string,
) error {
	key := teamAccessKey(organization, teamSlug, repositoryOwner, repositoryName)
	if _, ok := f.teamAccess[key]; !ok {
		return githubclient.ErrNotFound
	}
	f.removeTeamCalls++
	delete(f.teamAccess, key)
	return nil
}

func (f *fakeRepositoryAccessClient) GetCollaboratorAccess(
	_ context.Context,
	repositoryOwner string,
	repositoryName string,
	username string,
) (*githubclient.CollaboratorAccess, error) {
	access, ok := f.collaborators[collaboratorAccessKey(repositoryOwner, repositoryName, username)]
	if !ok {
		return nil, githubclient.ErrNotFound
	}
	copy := *access
	return &copy, nil
}

func (f *fakeRepositoryAccessClient) SetCollaboratorPermission(
	_ context.Context,
	repositoryOwner string,
	repositoryName string,
	username string,
	permission string,
) (*githubclient.CollaboratorAccess, error) {
	f.setCollaboratorCalls++
	access := &githubclient.CollaboratorAccess{Permission: permission}
	if f.inviteUsers[username] {
		access.InvitationPending = true
		access.InvitationID = int64(1000 + f.setCollaboratorCalls)
	}
	f.collaborators[collaboratorAccessKey(repositoryOwner, repositoryName, username)] = access
	copy := *access
	return &copy, nil
}

func (f *fakeRepositoryAccessClient) UpdateRepositoryInvitation(
	_ context.Context,
	repositoryOwner string,
	repositoryName string,
	invitationID int64,
	permission string,
) (*githubclient.CollaboratorAccess, error) {
	f.updateInvitationCalls++
	for key, access := range f.collaborators {
		if access.InvitationID == invitationID {
			access.Permission = permission
			f.collaborators[key] = access
			copy := *access
			return &copy, nil
		}
	}
	return nil, githubclient.ErrNotFound
}

func (f *fakeRepositoryAccessClient) RemoveCollaboratorAccess(
	_ context.Context,
	repositoryOwner string,
	repositoryName string,
	username string,
	_ int64,
) error {
	key := collaboratorAccessKey(repositoryOwner, repositoryName, username)
	if _, ok := f.collaborators[key]; !ok {
		return githubclient.ErrNotFound
	}
	f.removeCollaboratorCalls++
	delete(f.collaborators, key)
	return nil
}

func createRepositoryAccessDependencies(
	ctx context.Context,
	providerName string,
	secretName string,
	repositoryResourceName string,
	repositoryName string,
) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: "default"},
		Data:       map[string][]byte{"token": []byte("test-token")},
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

	repository := &githubv1alpha1.GitHubRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      repositoryResourceName,
			Namespace: "default",
		},
		Spec: githubv1alpha1.GitHubRepositorySpec{
			ProviderConfigRef: providerName,
			Name:              repositoryName,
		},
	}
	Expect(k8sClient.Create(ctx, repository)).To(Succeed())
}

func cleanupRepositoryAccessDependencies(
	ctx context.Context,
	providerName string,
	secretName string,
	repositoryResourceName string,
) {
	repository := &githubv1alpha1.GitHubRepository{}
	repositoryKey := types.NamespacedName{Name: repositoryResourceName, Namespace: "default"}
	if err := k8sClient.Get(ctx, repositoryKey, repository); err == nil {
		if controllerutil.ContainsFinalizer(repository, githubRepositoryFinalizer) {
			controllerutil.RemoveFinalizer(repository, githubRepositoryFinalizer)
			Expect(k8sClient.Update(ctx, repository)).To(Succeed())
		}
		Expect(k8sClient.Delete(ctx, repository)).To(Succeed())
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
	secretKey := types.NamespacedName{Name: secretName, Namespace: "default"}
	if err := k8sClient.Get(ctx, secretKey, secret); err == nil {
		Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
	}
}
