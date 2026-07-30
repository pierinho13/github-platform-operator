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
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCachedTokenProviderCreatesAndCachesInstallationToken(t *testing.T) {
	t.Parallel()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Method != http.MethodPost ||
			request.URL.Path != "/app/installations/123/access_tokens" {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("X-GitHub-Api-Version") != githubAPIVersion {
			t.Fatalf("unexpected API version %q", request.Header.Get("X-GitHub-Api-Version"))
		}

		jwt := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
		parts := strings.Split(jwt, ".")
		if len(parts) != 3 {
			t.Fatalf("unexpected JWT %q", jwt)
		}
		payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			t.Fatalf("decode JWT payload: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			t.Fatalf("decode JWT payload JSON: %v", err)
		}
		if payload["iss"] != "Iv1.test-client-id" {
			t.Fatalf("unexpected JWT issuer %#v", payload["iss"])
		}

		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(`{
			"token":"ghs_installation_token",
			"expires_at":"2099-01-01T00:00:00Z"
		}`))
	}))
	defer server.Close()

	provider := NewCachedTokenProvider(server.Client())
	authentication := Authentication{
		GitHubApp: &GitHubAppAuthentication{
			AppID:          "Iv1.test-client-id",
			InstallationID: 123,
			PrivateKeyPEM:  privateKeyPEM,
		},
	}

	first, err := provider.ResolveToken(context.Background(), authentication, server.URL)
	if err != nil {
		t.Fatalf("resolve first token: %v", err)
	}
	second, err := provider.ResolveToken(context.Background(), authentication, server.URL)
	if err != nil {
		t.Fatalf("resolve cached token: %v", err)
	}

	if first != "ghs_installation_token" || second != first {
		t.Fatalf("unexpected tokens %q and %q", first, second)
	}
	if requests.Load() != 1 {
		t.Fatalf("expected one installation-token request, got %d", requests.Load())
	}
}

func TestRateLimitTransportSharesReactiveBackoff(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writer.Header().Set("Retry-After", "60")
		writer.WriteHeader(http.StatusForbidden)
		_, _ = writer.Write([]byte(`{"message":"secondary rate limit exceeded"}`))
	}))
	defer server.Close()

	client := &http.Client{
		Timeout: time.Second,
		Transport: &RateLimitTransport{
			Base: server.Client().Transport,
			gate: &rateLimitGate{},
		},
	}

	_, err := client.Get(server.URL)
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	var firstRateLimitError *RateLimitError
	if !errors.As(err, &firstRateLimitError) {
		t.Fatalf("expected RateLimitError, got %T: %v", err, err)
	}

	_, err = client.Get(server.URL)
	if err == nil {
		t.Fatal("expected shared gate to reject the second request")
	}
	var secondRateLimitError *RateLimitError
	if !errors.As(err, &secondRateLimitError) {
		t.Fatalf("expected second RateLimitError, got %T: %v", err, err)
	}
	if requests.Load() != 1 {
		t.Fatalf("expected only one network request, got %d", requests.Load())
	}
}
