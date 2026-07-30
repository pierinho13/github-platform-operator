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
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCachedTokenProviderPATContract(t *testing.T) {
	t.Parallel()

	provider := NewCachedTokenProvider(nil)
	token, err := provider.ResolveToken(
		context.Background(),
		Authentication{Token: "  ghp_test-token  "},
		contractGitHubAPIURL,
	)
	if err != nil {
		t.Fatalf("resolve PAT: %v", err)
	}
	if token != "ghp_test-token" {
		t.Fatalf("expected trimmed PAT, got %q", token)
	}

	if _, err := provider.ResolveToken(
		context.Background(),
		Authentication{Token: "   "},
		contractGitHubAPIURL,
	); err == nil {
		t.Fatal("expected empty PAT to be rejected")
	}
}

func TestCachedTokenProviderGitHubAppValidationContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		auth Authentication
		want string
	}{
		{
			name: "empty app ID",
			auth: Authentication{GitHubApp: &GitHubAppAuthentication{
				InstallationID: 1,
				PrivateKeyPEM:  []byte("key"),
			}},
			want: "GitHub App ID must not be empty",
		},
		{
			name: "invalid installation ID",
			auth: Authentication{GitHubApp: &GitHubAppAuthentication{
				AppID:         "Iv1.test",
				PrivateKeyPEM: []byte("key"),
			}},
			want: "installation ID must be greater than zero",
		},
		{
			name: "empty private key",
			auth: Authentication{GitHubApp: &GitHubAppAuthentication{
				AppID:          "Iv1.test",
				InstallationID: 1,
			}},
			want: "private key must not be empty",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			provider := NewCachedTokenProvider(nil)
			_, err := provider.ResolveToken(context.Background(), test.auth, contractGitHubAPIURL)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestParseGitHubAppPrivateKeyContract(t *testing.T) {
	t.Parallel()

	privateKey := generateRSAKeyContractTest(t)
	pkcs1 := pem.EncodeToMemory(&pem.Block{
		Type:  testRSAPrivateKeyPEMType,
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal PKCS#8 key: %v", err)
	}
	pkcs8 := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8Bytes,
	})

	for name, value := range map[string][]byte{
		"PKCS1": pkcs1,
		"PKCS8": pkcs8,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			parsed, err := parseGitHubAppPrivateKey(value)
			if err != nil {
				t.Fatalf("parse private key: %v", err)
			}
			if parsed.N.Cmp(privateKey.N) != 0 {
				t.Fatal("parsed key does not match the original key")
			}
		})
	}

	if _, err := parseGitHubAppPrivateKey([]byte("not a PEM key")); err == nil {
		t.Fatal("expected invalid PEM to be rejected")
	}
}

func TestGenerateGitHubAppJWTContract(t *testing.T) {
	t.Parallel()

	privateKey := generateRSAKeyContractTest(t)
	now := time.Date(2026, time.July, 30, 20, 0, 0, 0, time.UTC)
	token, err := generateGitHubAppJWT(contractGitHubAppID, privateKey, now)
	if err != nil {
		t.Fatalf("generate JWT: %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected three JWT segments, got %d", len(parts))
	}

	var header map[string]any
	decodeJWTPartContractTest(t, parts[0], &header)
	if header["alg"] != "RS256" || header["typ"] != "JWT" {
		t.Fatalf("unexpected JWT header %#v", header)
	}

	var claims map[string]any
	decodeJWTPartContractTest(t, parts[1], &claims)
	if claims["iss"] != contractGitHubAppID {
		t.Fatalf("unexpected issuer %#v", claims["iss"])
	}
	if int64(claims["iat"].(float64)) != now.Add(-githubAppClockSkew).Unix() {
		t.Fatalf("unexpected iat %#v", claims["iat"])
	}
	if int64(claims["exp"].(float64)) != now.Add(githubAppJWTLifetime).Unix() {
		t.Fatalf("unexpected exp %#v", claims["exp"])
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode JWT signature: %v", err)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(&privateKey.PublicKey, crypto.SHA256, digest[:], signature); err != nil {
		t.Fatalf("verify JWT signature: %v", err)
	}
}

func TestCachedTokenProviderRefreshesNearExpiryContract(t *testing.T) {
	t.Parallel()

	privateKeyPEM := marshalPKCS1KeyContractTest(t, generateRSAKeyContractTest(t))
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requestNumber := requests.Add(1)
		expiresAt := time.Now().Add(4 * time.Minute)
		token := "short-lived"
		if requestNumber > 1 {
			expiresAt = time.Now().Add(time.Hour)
			token = "refreshed"
		}
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(`{"token":"` + token + `","expires_at":"` +
			expiresAt.UTC().Format(time.RFC3339) + `"}`))
	}))
	defer server.Close()

	provider := NewCachedTokenProvider(server.Client())
	authentication := Authentication{GitHubApp: &GitHubAppAuthentication{
		AppID:          contractGitHubAppID,
		InstallationID: 123,
		PrivateKeyPEM:  privateKeyPEM,
	}}

	first, err := provider.ResolveToken(context.Background(), authentication, server.URL)
	if err != nil {
		t.Fatalf("resolve first token: %v", err)
	}
	second, err := provider.ResolveToken(context.Background(), authentication, server.URL)
	if err != nil {
		t.Fatalf("resolve refreshed token: %v", err)
	}
	if first != "short-lived" || second != "refreshed" {
		t.Fatalf("unexpected tokens %q and %q", first, second)
	}
	if requests.Load() != 2 {
		t.Fatalf("expected two installation-token requests, got %d", requests.Load())
	}
}

func TestCachedTokenProviderDeduplicatesConcurrentRefreshContract(t *testing.T) {
	t.Parallel()

	privateKeyPEM := marshalPKCS1KeyContractTest(t, generateRSAKeyContractTest(t))
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		time.Sleep(25 * time.Millisecond)
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(`{"token":"shared-token","expires_at":"` + contractFarFutureExpiration + `"}`))
	}))
	defer server.Close()

	provider := NewCachedTokenProvider(server.Client())
	authentication := Authentication{GitHubApp: &GitHubAppAuthentication{
		AppID:          contractGitHubAppID,
		InstallationID: 123,
		PrivateKeyPEM:  privateKeyPEM,
	}}

	const workers = 12
	results := make(chan string, workers)
	errorsChannel := make(chan error, workers)
	var waitGroup sync.WaitGroup
	waitGroup.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer waitGroup.Done()
			token, err := provider.ResolveToken(context.Background(), authentication, server.URL)
			if err != nil {
				errorsChannel <- err
				return
			}
			results <- token
		}()
	}
	waitGroup.Wait()
	close(results)
	close(errorsChannel)

	for err := range errorsChannel {
		t.Fatalf("resolve token concurrently: %v", err)
	}
	for token := range results {
		if token != "shared-token" {
			t.Fatalf("unexpected token %q", token)
		}
	}
	if requests.Load() != 1 {
		t.Fatalf("expected one installation-token request, got %d", requests.Load())
	}
}

func TestInstallationTokenCacheKeyIncludesPrivateKeyContract(t *testing.T) {
	t.Parallel()

	firstKey := marshalPKCS1KeyContractTest(t, generateRSAKeyContractTest(t))
	secondKey := marshalPKCS1KeyContractTest(t, generateRSAKeyContractTest(t))
	first := installationTokenCacheKey(contractGitHubAPIURL, &GitHubAppAuthentication{
		AppID:          contractGitHubAppID,
		InstallationID: 123,
		PrivateKeyPEM:  firstKey,
	})
	second := installationTokenCacheKey(contractGitHubAPIURL, &GitHubAppAuthentication{
		AppID:          contractGitHubAppID,
		InstallationID: 123,
		PrivateKeyPEM:  secondKey,
	})
	if first == second {
		t.Fatal("rotating the private key must change the installation-token cache key")
	}
}

func TestRateLimitResponseDetectionContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		headers    map[string]string
		body       string
		wantRate   bool
		minDelay   time.Duration
		maxDelay   time.Duration
	}{
		{
			name:       "429 without headers uses fallback",
			statusCode: http.StatusTooManyRequests,
			body:       `{"message":"slow down"}`,
			wantRate:   true,
			minDelay:   time.Minute,
			maxDelay:   time.Minute + 2*time.Second,
		},
		{
			name:       "retry after seconds",
			statusCode: http.StatusForbidden,
			headers:    map[string]string{"Retry-After": "7"},
			body:       `{"message":"secondary rate limit exceeded"}`,
			wantRate:   true,
			minDelay:   7 * time.Second,
			maxDelay:   9 * time.Second,
		},
		{
			name:       "abuse detection body",
			statusCode: http.StatusForbidden,
			body:       `{"message":"abuse detection mechanism"}`,
			wantRate:   true,
			minDelay:   time.Minute,
			maxDelay:   time.Minute + 2*time.Second,
		},
		{
			name:       "ordinary forbidden response is not rate limited",
			statusCode: http.StatusForbidden,
			body:       `{"message":"Resource not accessible by personal access token"}`,
			wantRate:   false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			response := &http.Response{
				StatusCode: test.statusCode,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(test.body)),
			}
			for key, value := range test.headers {
				response.Header.Set(key, value)
			}
			rateErr, err := rateLimitErrorFromResponse(response)
			if err != nil {
				t.Fatalf("detect rate limit: %v", err)
			}
			if !test.wantRate {
				if rateErr != nil {
					t.Fatalf("did not expect rate limit error: %v", rateErr)
				}
				body, readErr := io.ReadAll(response.Body)
				if readErr != nil {
					t.Fatalf("read restored response body: %v", readErr)
				}
				if string(body) != test.body {
					t.Fatalf("response body was not restored: %q", body)
				}
				return
			}
			if rateErr == nil {
				t.Fatal("expected rate limit error")
			}
			delay := time.Until(rateErr.RetryAt)
			if delay < test.minDelay || delay > test.maxDelay {
				t.Fatalf("unexpected retry delay %s, expected between %s and %s", delay, test.minDelay, test.maxDelay)
			}
			if rateErr.Body != test.body {
				t.Fatalf("unexpected rate-limit body %q", rateErr.Body)
			}
		})
	}
}

func TestPrimaryRateLimitResetContract(t *testing.T) {
	t.Parallel()

	resetAt := time.Now().Add(10 * time.Second).Unix()
	response := &http.Response{
		StatusCode: http.StatusForbidden,
		Header: http.Header{
			"X-Ratelimit-Remaining": []string{"0"},
			"X-Ratelimit-Reset":     []string{strconv.FormatInt(resetAt, 10)},
		},
		Body: io.NopCloser(strings.NewReader(`{"message":"API rate limit exceeded"}`)),
	}
	rateErr, err := rateLimitErrorFromResponse(response)
	if err != nil {
		t.Fatalf("detect primary rate limit: %v", err)
	}
	if rateErr == nil {
		t.Fatal("expected primary rate limit error")
	}
	expected := time.Unix(resetAt, 0).Add(time.Second)
	if delta := rateErr.RetryAt.Sub(expected); delta < -time.Millisecond || delta > time.Millisecond {
		t.Fatalf("unexpected retry time %s, expected %s", rateErr.RetryAt, expected)
	}
}

func TestRateLimitErrorRetryAfterContract(t *testing.T) {
	t.Parallel()

	if delay := (*RateLimitError)(nil).RetryAfter(); delay != fallbackSecondaryRateLimitDelay {
		t.Fatalf("unexpected nil error retry delay %s", delay)
	}
	if delay := (&RateLimitError{}).RetryAfter(); delay != fallbackSecondaryRateLimitDelay {
		t.Fatalf("unexpected zero retry delay %s", delay)
	}
	if delay := (&RateLimitError{RetryAt: time.Now().Add(-time.Minute)}).RetryAfter(); delay != time.Second {
		t.Fatalf("expected one-second floor, got %s", delay)
	}
}

func TestRateLimitGateKeepsLongestBackoffContract(t *testing.T) {
	t.Parallel()

	gate := &rateLimitGate{}
	now := time.Now()
	gate.block(now.Add(time.Minute), http.StatusForbidden, "first")
	gate.block(now.Add(10*time.Second), http.StatusTooManyRequests, "shorter")

	rateErr := gate.currentError(now)
	if rateErr == nil {
		t.Fatal("expected gate to be blocked")
	}
	if rateErr.Body != "first" || rateErr.StatusCode != http.StatusForbidden {
		t.Fatalf("shorter backoff replaced longer backoff: %#v", rateErr)
	}
	if gate.currentError(now.Add(2*time.Minute)) != nil {
		t.Fatal("expected expired gate to allow requests")
	}
}

func TestRateLimitTransportAllowsOrdinaryForbiddenResponsesContract(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writer.WriteHeader(http.StatusForbidden)
		_, _ = writer.Write([]byte(`{"message":"forbidden"}`))
	}))
	defer server.Close()

	client := &http.Client{
		Transport: &RateLimitTransport{
			Base: server.Client().Transport,
			gate: &rateLimitGate{},
		},
	}
	for i := 0; i < 2; i++ {
		response, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("ordinary 403 should be returned as a response: %v", err)
		}
		closeResponseBody(response.Body)
	}
	if requests.Load() != 2 {
		t.Fatalf("expected both requests to reach the server, got %d", requests.Load())
	}
}

func generateRSAKeyContractTest(t *testing.T) *rsa.PrivateKey {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return privateKey
}

func marshalPKCS1KeyContractTest(t *testing.T, privateKey *rsa.PrivateKey) []byte {
	t.Helper()

	return pem.EncodeToMemory(&pem.Block{
		Type:  testRSAPrivateKeyPEMType,
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
}

func decodeJWTPartContractTest(t *testing.T, encoded string, target any) {
	t.Helper()

	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode JWT segment: %v", err)
	}
	if err := json.Unmarshal(decoded, target); err != nil {
		t.Fatalf("decode JWT JSON: %v", err)
	}
}

func TestNormalizeAPIURLContract(t *testing.T) {
	t.Parallel()

	normalized, err := normalizeAPIURL("  https://api.github.com/  ")
	if err != nil {
		t.Fatalf("normalize API URL: %v", err)
	}
	if normalized != contractGitHubAPIURL {
		t.Fatalf("unexpected normalized URL %q", normalized)
	}

	for _, value := range []string{"", "api.github.com", "ftp://api.github.com"} {
		if _, err := normalizeAPIURL(value); err == nil {
			t.Fatalf("expected invalid API URL %q to be rejected", value)
		}
	}
}

func TestInstallationTokenErrorContract(t *testing.T) {
	t.Parallel()

	privateKeyPEM := marshalPKCS1KeyContractTest(t, generateRSAKeyContractTest(t))
	tests := []struct {
		name       string
		statusCode int
		body       string
		want       string
	}{
		{
			name:       "GitHub API error",
			statusCode: http.StatusUnauthorized,
			body:       `{"message":"Bad credentials"}`,
			want:       "Bad credentials",
		},
		{
			name:       "empty token",
			statusCode: http.StatusCreated,
			body:       `{"token":"","expires_at":"` + contractFarFutureExpiration + `"}`,
			want:       "empty installation token",
		},
		{
			name:       "invalid expiration",
			statusCode: http.StatusCreated,
			body:       `{"token":"token","expires_at":"not-a-date"}`,
			want:       "parse GitHub App installation token expiration",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.statusCode)
				_, _ = writer.Write([]byte(test.body))
			}))
			defer server.Close()

			provider := NewCachedTokenProvider(server.Client())
			_, err := provider.ResolveToken(context.Background(), Authentication{
				GitHubApp: &GitHubAppAuthentication{
					AppID:          contractGitHubAppID,
					InstallationID: 123,
					PrivateKeyPEM:  privateKeyPEM,
				},
			}, server.URL)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestRateLimitErrorCanBeUnwrappedContract(t *testing.T) {
	t.Parallel()

	expected := &RateLimitError{RetryAt: time.Now().Add(time.Minute)}
	wrapped := errors.Join(errors.New("reconcile failed"), expected)
	var observed *RateLimitError
	if !errors.As(wrapped, &observed) || observed != expected {
		t.Fatalf("expected errors.As to recover RateLimitError, got %#v", observed)
	}
}
