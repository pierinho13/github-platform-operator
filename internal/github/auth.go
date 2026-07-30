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
	"bytes"
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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	githubAPIVersion          = "2026-03-10"
	githubAppJWTLifetime      = 9 * time.Minute
	githubAppClockSkew        = time.Minute
	installationRefreshMargin = 5 * time.Minute
)

// Authentication is either an already-issued token or GitHub App credentials.
type Authentication struct {
	Token     string
	GitHubApp *GitHubAppAuthentication
}

// GitHubAppAuthentication contains the values required to mint an installation token.
type GitHubAppAuthentication struct {
	AppID          string
	InstallationID int64
	PrivateKeyPEM  []byte
}

// TokenProvider resolves an authentication configuration into a GitHub API token.
type TokenProvider interface {
	ResolveToken(ctx context.Context, authentication Authentication, baseURL string) (string, error)
}

type cachedInstallationToken struct {
	token     string
	expiresAt time.Time
}

// CachedTokenProvider mints and caches GitHub App installation tokens.
// Personal access tokens pass through without caching.
type CachedTokenProvider struct {
	httpClient *http.Client

	mu    sync.Mutex
	cache map[string]cachedInstallationToken
}

// NewCachedTokenProvider creates a provider. Supplying the same shared HTTP
// client used by RESTClientFactory also shares the global rate-limit gate.
func NewCachedTokenProvider(httpClient *http.Client) *CachedTokenProvider {
	if httpClient == nil {
		httpClient = NewRateLimitedHTTPClient()
	}
	return &CachedTokenProvider{
		httpClient: httpClient,
		cache:      make(map[string]cachedInstallationToken),
	}
}

// ResolveToken returns the configured token or a cached GitHub App installation token.
func (p *CachedTokenProvider) ResolveToken(
	ctx context.Context,
	authentication Authentication,
	baseURL string,
) (string, error) {
	if p == nil {
		return "", errors.New("GitHub token provider is nil")
	}

	if authentication.GitHubApp == nil {
		token := strings.TrimSpace(authentication.Token)
		if token == "" {
			return "", errors.New("GitHub token must not be empty")
		}
		return token, nil
	}

	app := authentication.GitHubApp
	if strings.TrimSpace(app.AppID) == "" {
		return "", errors.New("GitHub App ID must not be empty")
	}
	if app.InstallationID <= 0 {
		return "", errors.New("GitHub App installation ID must be greater than zero")
	}
	if len(bytes.TrimSpace(app.PrivateKeyPEM)) == 0 {
		return "", errors.New("GitHub App private key must not be empty")
	}

	baseURL, err := normalizeAPIURL(baseURL)
	if err != nil {
		return "", err
	}

	cacheKey := installationTokenCacheKey(baseURL, app)

	// Holding the mutex while minting prevents a startup burst from generating
	// one installation token per reconciler.
	p.mu.Lock()
	defer p.mu.Unlock()

	if cached, ok := p.cache[cacheKey]; ok &&
		time.Now().Add(installationRefreshMargin).Before(cached.expiresAt) {
		return cached.token, nil
	}

	token, expiresAt, err := p.createInstallationToken(ctx, baseURL, app)
	if err != nil {
		return "", err
	}

	p.cache[cacheKey] = cachedInstallationToken{
		token:     token,
		expiresAt: expiresAt,
	}
	return token, nil
}

func (p *CachedTokenProvider) createInstallationToken(
	ctx context.Context,
	baseURL string,
	app *GitHubAppAuthentication,
) (string, time.Time, error) {
	privateKey, err := parseGitHubAppPrivateKey(app.PrivateKeyPEM)
	if err != nil {
		return "", time.Time{}, err
	}

	jwt, err := generateGitHubAppJWT(strings.TrimSpace(app.AppID), privateKey, time.Now())
	if err != nil {
		return "", time.Time{}, err
	}

	endpoint := fmt.Sprintf(
		"%s/app/installations/%d/access_tokens",
		baseURL,
		app.InstallationID,
	)
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader([]byte("{}")),
	)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("create GitHub App installation token request: %w", err)
	}

	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+jwt)
	request.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	request.Header.Set("User-Agent", "github-platform-operator")
	request.Header.Set("Content-Type", "application/json")

	response, err := p.httpClient.Do(request)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("request GitHub App installation token: %w", err)
	}
	defer closeResponseBody(response.Body)

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", time.Time{}, decodeAPIError(response)
	}

	var payload struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxResponseBodySize)).Decode(&payload); err != nil {
		return "", time.Time{}, fmt.Errorf("decode GitHub App installation token response: %w", err)
	}

	payload.Token = strings.TrimSpace(payload.Token)
	if payload.Token == "" {
		return "", time.Time{}, errors.New("GitHub returned an empty installation token")
	}
	expiresAt, err := time.Parse(time.RFC3339, payload.ExpiresAt)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("parse GitHub App installation token expiration: %w", err)
	}

	return payload.Token, expiresAt, nil
}

func normalizeAPIURL(value string) (string, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	parsedURL, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("parse GitHub API URL: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return "", errors.New("GitHub API URL must use http or https")
	}
	if parsedURL.Host == "" {
		return "", errors.New("GitHub API URL must include a host")
	}
	return value, nil
}

func installationTokenCacheKey(baseURL string, app *GitHubAppAuthentication) string {
	keyHash := sha256.Sum256(app.PrivateKeyPEM)
	return strings.Join(
		[]string{
			baseURL,
			strings.TrimSpace(app.AppID),
			strconv.FormatInt(app.InstallationID, 10),
			base64.RawURLEncoding.EncodeToString(keyHash[:]),
		},
		"|",
	)
}

func parseGitHubAppPrivateKey(privateKeyPEM []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return nil, errors.New("decode GitHub App private key PEM: no PEM block found")
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse GitHub App private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("GitHub App private key is not an RSA private key")
	}
	return key, nil
}

func generateGitHubAppJWT(
	appID string,
	privateKey *rsa.PrivateKey,
	now time.Time,
) (string, error) {
	header, err := json.Marshal(map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	})
	if err != nil {
		return "", fmt.Errorf("encode GitHub App JWT header: %w", err)
	}

	payload, err := json.Marshal(map[string]any{
		"iat": now.Add(-githubAppClockSkew).Unix(),
		"exp": now.Add(githubAppJWTLifetime).Unix(),
		"iss": appID,
	})
	if err != nil {
		return "", fmt.Errorf("encode GitHub App JWT payload: %w", err)
	}

	encodedHeader := base64.RawURLEncoding.EncodeToString(header)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signingInput := encodedHeader + "." + encodedPayload
	digest := sha256.Sum256([]byte(signingInput))

	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign GitHub App JWT: %w", err)
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}
