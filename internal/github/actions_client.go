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

package github

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/nacl/box"
)

// ActionsTargetScope identifies a GitHub Actions target.
type ActionsTargetScope string

const (
	ActionsTargetScopeRepository   ActionsTargetScope = "repository"
	ActionsTargetScopeEnvironment  ActionsTargetScope = "environment"
	ActionsTargetScopeOrganization ActionsTargetScope = "organization"
)

// ActionsTarget contains the resolved GitHub coordinates for an Actions resource.
type ActionsTarget struct {
	Scope        ActionsTargetScope
	Organization string
	Repository   string
	Environment  string
}

// Environment contains the GitHub fields required by the environment controller.
type Environment struct {
	ID        int64
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ActionsPublicKey is the public key used to encrypt an Actions secret.
type ActionsPublicKey struct {
	KeyID string
	Key   string
}

// ActionsSecret contains the metadata GitHub exposes for a secret.
type ActionsSecret struct {
	Name                  string
	Visibility            string
	SelectedRepositoryIDs []int64
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// ActionsSecretUpsert contains the encrypted secret and optional organization policy.
type ActionsSecretUpsert struct {
	EncryptedValue        string  `json:"encrypted_value"`
	KeyID                 string  `json:"key_id"`
	Visibility            *string `json:"visibility,omitempty"`
	SelectedRepositoryIDs []int64 `json:"selected_repository_ids,omitempty"`
}

// ActionsVariable contains a GitHub Actions variable.
type ActionsVariable struct {
	Name                  string
	Value                 string
	Visibility            string
	SelectedRepositoryIDs []int64
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// ActionsVariableUpsert contains a variable and optional organization policy.
type ActionsVariableUpsert struct {
	Name                  string  `json:"name"`
	Value                 string  `json:"value"`
	Visibility            *string `json:"visibility,omitempty"`
	SelectedRepositoryIDs []int64 `json:"selected_repository_ids,omitempty"`
}

// ActionsClientFactory creates clients used to manage environments, secrets and variables.
type ActionsClientFactory interface {
	NewActionsClient(token, baseURL string) (ActionsClient, error)
}

// NewActionsClient creates a REST-backed Actions client.
func (f RESTClientFactory) NewActionsClient(token, baseURL string) (ActionsClient, error) {
	return NewRESTClientWithHTTPClient(token, baseURL, f.HTTPClient)
}

// ActionsClient defines the GitHub operations used by the Actions controllers.
type ActionsClient interface {
	GetRepository(ctx context.Context, organization, name string) (*Repository, error)

	GetEnvironment(ctx context.Context, organization, repository, environment string) (*Environment, error)
	UpsertEnvironment(ctx context.Context, organization, repository, environment string) (*Environment, error)
	DeleteEnvironment(ctx context.Context, organization, repository, environment string) error

	GetActionsPublicKey(ctx context.Context, target ActionsTarget) (*ActionsPublicKey, error)
	GetActionsSecret(ctx context.Context, target ActionsTarget, name string) (*ActionsSecret, error)
	UpsertActionsSecret(ctx context.Context, target ActionsTarget, name string, input ActionsSecretUpsert) error
	DeleteActionsSecret(ctx context.Context, target ActionsTarget, name string) error

	GetActionsVariable(ctx context.Context, target ActionsTarget, name string) (*ActionsVariable, error)
	CreateActionsVariable(ctx context.Context, target ActionsTarget, input ActionsVariableUpsert) error
	UpdateActionsVariable(ctx context.Context, target ActionsTarget, name string, input ActionsVariableUpsert) error
	DeleteActionsVariable(ctx context.Context, target ActionsTarget, name string) error
}

// EncryptActionsSecret encrypts a value using GitHub's Base64-encoded Curve25519
// public key and an anonymous sealed box compatible with libsodium crypto_box_seal.
func EncryptActionsSecret(publicKey string, value []byte) (string, error) {
	decodedKey, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil {
		return "", fmt.Errorf("decode GitHub Actions public key: %w", err)
	}
	if len(decodedKey) != 32 {
		return "", fmt.Errorf("GitHub Actions public key must contain 32 bytes, got %d", len(decodedKey))
	}

	var recipient [32]byte
	copy(recipient[:], decodedKey)

	encrypted, err := box.SealAnonymous(nil, value, &recipient, rand.Reader)
	if err != nil {
		return "", fmt.Errorf("encrypt GitHub Actions secret: %w", err)
	}
	if len(encrypted) == 0 {
		return "", errors.New("encrypted GitHub Actions secret is empty")
	}

	return base64.StdEncoding.EncodeToString(encrypted), nil
}
