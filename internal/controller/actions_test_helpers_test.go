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
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"golang.org/x/crypto/nacl/box"

	githubclient "github.com/pierinho13/github-platform-operator/internal/github"
)

type fakeActionsClientFactory struct {
	client     *fakeActionsClient
	lastToken  string
	lastAPIURL string
}

func (f *fakeActionsClientFactory) NewActionsClient(
	token string,
	baseURL string,
) (githubclient.ActionsClient, error) {
	f.lastToken = token
	f.lastAPIURL = baseURL
	return f.client, nil
}

type fakeActionsClient struct {
	repositories map[string]*githubclient.Repository
	environments map[string]*githubclient.Environment
	secrets      map[string]*githubclient.ActionsSecret
	variables    map[string]*githubclient.ActionsVariable
	publicKey    *githubclient.ActionsPublicKey

	upsertEnvironmentCalls int
	deleteEnvironmentCalls int
	upsertSecretCalls      int
	deleteSecretCalls      int
	createVariableCalls    int
	updateVariableCalls    int
	deleteVariableCalls    int
}

func newFakeActionsClient() *fakeActionsClient {
	publicKey, _, err := box.GenerateKey(rand.Reader)
	if err != nil {
		panic(fmt.Sprintf("generate test public key: %v", err))
	}
	return &fakeActionsClient{
		repositories: make(map[string]*githubclient.Repository),
		environments: make(map[string]*githubclient.Environment),
		secrets:      make(map[string]*githubclient.ActionsSecret),
		variables:    make(map[string]*githubclient.ActionsVariable),
		publicKey: &githubclient.ActionsPublicKey{
			KeyID: "test-key",
			Key:   base64.StdEncoding.EncodeToString(publicKey[:]),
		},
	}
}

func (f *fakeActionsClient) GetRepository(
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

func environmentKey(organization, repository, environment string) string {
	return organization + "/" + repository + "/" + environment
}

func actionsTargetKey(target githubclient.ActionsTarget, name string) string {
	return fmt.Sprintf(
		"%s/%s/%s/%s/%s",
		target.Scope,
		target.Organization,
		target.Repository,
		target.Environment,
		name,
	)
}

func (f *fakeActionsClient) GetEnvironment(
	_ context.Context,
	organization string,
	repository string,
	environment string,
) (*githubclient.Environment, error) {
	item, ok := f.environments[environmentKey(organization, repository, environment)]
	if !ok {
		return nil, githubclient.ErrNotFound
	}
	copy := *item
	return &copy, nil
}

func (f *fakeActionsClient) UpsertEnvironment(
	_ context.Context,
	organization string,
	repository string,
	environment string,
) (*githubclient.Environment, error) {
	f.upsertEnvironmentCalls++
	now := time.Now().UTC()
	item := &githubclient.Environment{
		ID:        int64(f.upsertEnvironmentCalls),
		Name:      environment,
		CreatedAt: now,
		UpdatedAt: now,
	}
	f.environments[environmentKey(organization, repository, environment)] = item
	copy := *item
	return &copy, nil
}

func (f *fakeActionsClient) DeleteEnvironment(
	_ context.Context,
	organization string,
	repository string,
	environment string,
) error {
	key := environmentKey(organization, repository, environment)
	if _, ok := f.environments[key]; !ok {
		return githubclient.ErrNotFound
	}
	f.deleteEnvironmentCalls++
	delete(f.environments, key)
	return nil
}

func (f *fakeActionsClient) GetActionsPublicKey(
	_ context.Context,
	_ githubclient.ActionsTarget,
) (*githubclient.ActionsPublicKey, error) {
	copy := *f.publicKey
	return &copy, nil
}

func (f *fakeActionsClient) GetActionsSecret(
	_ context.Context,
	target githubclient.ActionsTarget,
	name string,
) (*githubclient.ActionsSecret, error) {
	item, ok := f.secrets[actionsTargetKey(target, name)]
	if !ok {
		return nil, githubclient.ErrNotFound
	}
	copy := *item
	copy.SelectedRepositoryIDs = append([]int64(nil), item.SelectedRepositoryIDs...)
	return &copy, nil
}

func (f *fakeActionsClient) UpsertActionsSecret(
	_ context.Context,
	target githubclient.ActionsTarget,
	name string,
	input githubclient.ActionsSecretUpsert,
) error {
	f.upsertSecretCalls++
	now := time.Now().UTC()
	visibility := ""
	if input.Visibility != nil {
		visibility = *input.Visibility
	}
	f.secrets[actionsTargetKey(target, name)] = &githubclient.ActionsSecret{
		Name:                  name,
		Visibility:            visibility,
		SelectedRepositoryIDs: append([]int64(nil), input.SelectedRepositoryIDs...),
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	return nil
}

func (f *fakeActionsClient) DeleteActionsSecret(
	_ context.Context,
	target githubclient.ActionsTarget,
	name string,
) error {
	key := actionsTargetKey(target, name)
	if _, ok := f.secrets[key]; !ok {
		return githubclient.ErrNotFound
	}
	f.deleteSecretCalls++
	delete(f.secrets, key)
	return nil
}

func (f *fakeActionsClient) GetActionsVariable(
	_ context.Context,
	target githubclient.ActionsTarget,
	name string,
) (*githubclient.ActionsVariable, error) {
	item, ok := f.variables[actionsTargetKey(target, name)]
	if !ok {
		return nil, githubclient.ErrNotFound
	}
	copy := *item
	copy.SelectedRepositoryIDs = append([]int64(nil), item.SelectedRepositoryIDs...)
	return &copy, nil
}

func (f *fakeActionsClient) CreateActionsVariable(
	_ context.Context,
	target githubclient.ActionsTarget,
	input githubclient.ActionsVariableUpsert,
) error {
	f.createVariableCalls++
	return f.storeVariable(target, input)
}

func (f *fakeActionsClient) UpdateActionsVariable(
	_ context.Context,
	target githubclient.ActionsTarget,
	_ string,
	input githubclient.ActionsVariableUpsert,
) error {
	f.updateVariableCalls++
	return f.storeVariable(target, input)
}

func (f *fakeActionsClient) storeVariable(
	target githubclient.ActionsTarget,
	input githubclient.ActionsVariableUpsert,
) error {
	now := time.Now().UTC()
	visibility := ""
	if input.Visibility != nil {
		visibility = *input.Visibility
	}
	f.variables[actionsTargetKey(target, input.Name)] = &githubclient.ActionsVariable{
		Name:                  input.Name,
		Value:                 input.Value,
		Visibility:            visibility,
		SelectedRepositoryIDs: append([]int64(nil), input.SelectedRepositoryIDs...),
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	return nil
}

func (f *fakeActionsClient) DeleteActionsVariable(
	_ context.Context,
	target githubclient.ActionsTarget,
	name string,
) error {
	key := actionsTargetKey(target, name)
	if _, ok := f.variables[key]; !ok {
		return githubclient.ErrNotFound
	}
	f.deleteVariableCalls++
	delete(f.variables, key)
	return nil
}
