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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/crypto/nacl/box"
)

func TestEncryptActionsSecret(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := box.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	plaintext := []byte("super-secret-value")
	encrypted, err := EncryptActionsSecret(
		base64.StdEncoding.EncodeToString(publicKey[:]),
		plaintext,
	)
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		t.Fatalf("decode encrypted value: %v", err)
	}
	decrypted, ok := box.OpenAnonymous(nil, ciphertext, publicKey, privateKey)
	if !ok {
		t.Fatal("could not decrypt anonymous sealed box")
	}
	if string(decrypted) != string(plaintext) {
		t.Fatalf("unexpected plaintext %q", decrypted)
	}
}

func TestRepositoryActionsSecretEndpoints(t *testing.T) {
	t.Parallel()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		switch requests {
		case 1:
			if request.Method != http.MethodGet ||
				request.URL.Path != "/repos/k8sready/example/actions/secrets/public-key" {
				t.Fatalf("unexpected public-key request %s %s", request.Method, request.URL.Path)
			}
			_, _ = writer.Write([]byte(`{"key_id":"key-1","key":"ZmFrZS1rZXk="}`))
		case 2:
			if request.Method != http.MethodPut ||
				request.URL.Path != "/repos/k8sready/example/actions/secrets/DOCKER_TOKEN" {
				t.Fatalf("unexpected secret request %s %s", request.Method, request.URL.Path)
			}
			var body ActionsSecretUpsert
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if body.KeyID != "key-1" || body.EncryptedValue != "encrypted" {
				t.Fatalf("unexpected request body %#v", body)
			}
			writer.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected extra request %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewRESTClient("token", server.URL)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	target := ActionsTarget{
		Scope:        ActionsTargetScopeRepository,
		Organization: rulesetActorTestOrganization,
		Repository:   "example",
	}
	key, err := client.GetActionsPublicKey(context.Background(), target)
	if err != nil {
		t.Fatalf("get public key: %v", err)
	}
	if key.KeyID != "key-1" {
		t.Fatalf("unexpected key ID %q", key.KeyID)
	}
	if err := client.UpsertActionsSecret(context.Background(), target, "DOCKER_TOKEN", ActionsSecretUpsert{
		EncryptedValue: "encrypted",
		KeyID:          key.KeyID,
	}); err != nil {
		t.Fatalf("upsert secret: %v", err)
	}
}

func TestEnvironmentAndOrganizationVariableEndpoints(t *testing.T) {
	t.Parallel()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		switch requests {
		case 1:
			if request.Method != http.MethodPut ||
				request.URL.Path != "/repos/k8sready/example/environments/production" {
				t.Fatalf("unexpected environment request %s %s", request.Method, request.URL.Path)
			}
			_, _ = writer.Write([]byte(`{"id":42,"name":"production"}`))
		case 2:
			if request.Method != http.MethodPost ||
				request.URL.Path != "/orgs/k8sready/actions/variables" {
				t.Fatalf("unexpected variable request %s %s", request.Method, request.URL.Path)
			}
			var body ActionsVariableUpsert
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if body.Name != "CLOUD_REGION" || body.Value != "eu-west-1" ||
				body.Visibility == nil || *body.Visibility != "selected" ||
				len(body.SelectedRepositoryIDs) != 1 || body.SelectedRepositoryIDs[0] != 123 {
				t.Fatalf("unexpected variable body %#v", body)
			}
			writer.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected extra request %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewRESTClient("token", server.URL)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	environment, err := client.UpsertEnvironment(
		context.Background(), rulesetActorTestOrganization, "example", "production",
	)
	if err != nil {
		t.Fatalf("upsert environment: %v", err)
	}
	if environment.ID != 42 || environment.Name != "production" {
		t.Fatalf("unexpected environment %#v", environment)
	}

	visibility := "selected"
	if err := client.CreateActionsVariable(
		context.Background(),
		ActionsTarget{Scope: ActionsTargetScopeOrganization, Organization: rulesetActorTestOrganization},
		ActionsVariableUpsert{
			Name:                  "CLOUD_REGION",
			Value:                 "eu-west-1",
			Visibility:            &visibility,
			SelectedRepositoryIDs: []int64{123},
		},
	); err != nil {
		t.Fatalf("create organization variable: %v", err)
	}
}
