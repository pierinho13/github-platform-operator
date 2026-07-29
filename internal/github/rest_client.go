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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultRequestTimeout = 30 * time.Second
	maxResponseBodySize   = 1 << 20
)

// RESTClient is a minimal GitHub REST API client.
type RESTClient struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

// APIError describes an unsuccessful response from the GitHub API.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("GitHub API returned status %d: %s", e.StatusCode, e.Body)
}

// NewRESTClient creates a GitHub REST API client.
func NewRESTClient(token, baseURL string) (*RESTClient, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("GitHub token must not be empty")
	}

	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse GitHub API URL: %w", err)
	}
	if parsedURL.Scheme != "https" && parsedURL.Scheme != "http" {
		return nil, fmt.Errorf("GitHub API URL must use http or https")
	}
	if parsedURL.Host == "" {
		return nil, fmt.Errorf("GitHub API URL must include a host")
	}

	return &RESTClient{
		token:   token,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: defaultRequestTimeout,
		},
	}, nil
}

// GetRepository returns a GitHub repository.
func (c *RESTClient) GetRepository(
	ctx context.Context,
	organization string,
	name string,
) (*Repository, error) {
	endpoint := c.repositoryEndpoint(organization, name)

	response, err := c.do(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, decodeAPIError(response)
	}

	return decodeRepository(response.Body)
}

// CreateRepository creates a repository in a GitHub organization.
func (c *RESTClient) CreateRepository(
	ctx context.Context,
	organization string,
	name string,
	private bool,
) (*Repository, error) {
	requestBody, err := json.Marshal(struct {
		Name    string `json:"name"`
		Private bool   `json:"private"`
	}{
		Name:    name,
		Private: private,
	})
	if err != nil {
		return nil, fmt.Errorf("encode create repository request: %w", err)
	}

	endpoint := fmt.Sprintf(
		"%s/orgs/%s/repos",
		c.baseURL,
		url.PathEscape(organization),
	)

	response, err := c.do(ctx, http.MethodPost, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, decodeAPIError(response)
	}

	return decodeRepository(response.Body)
}

// UpdateRepositoryVisibility changes an existing repository visibility.
func (c *RESTClient) UpdateRepositoryVisibility(
	ctx context.Context,
	organization string,
	name string,
	visibility string,
) (*Repository, error) {
	requestBody, err := json.Marshal(struct {
		Visibility string `json:"visibility"`
	}{
		Visibility: visibility,
	})
	if err != nil {
		return nil, fmt.Errorf("encode update repository request: %w", err)
	}

	response, err := c.do(
		ctx,
		http.MethodPatch,
		c.repositoryEndpoint(organization, name),
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, decodeAPIError(response)
	}

	return decodeRepository(response.Body)
}

// DeleteRepository deletes an existing GitHub repository.
func (c *RESTClient) DeleteRepository(
	ctx context.Context,
	organization string,
	name string,
) error {
	response, err := c.do(
		ctx,
		http.MethodDelete,
		c.repositoryEndpoint(organization, name),
		nil,
	)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return decodeAPIError(response)
	}

	return nil
}

func (c *RESTClient) repositoryEndpoint(organization, name string) string {
	return fmt.Sprintf(
		"%s/repos/%s/%s",
		c.baseURL,
		url.PathEscape(organization),
		url.PathEscape(name),
	)
}

func (c *RESTClient) do(
	ctx context.Context,
	method string,
	endpoint string,
	body io.Reader,
) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("create GitHub API request: %w", err)
	}

	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "github-platform-operator")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("execute GitHub API request: %w", err)
	}

	return response, nil
}

func decodeRepository(body io.Reader) (*Repository, error) {
	var payload struct {
		ID         int64  `json:"id"`
		HTMLURL    string `json:"html_url"`
		Visibility string `json:"visibility"`
		Private    bool   `json:"private"`
	}

	if err := json.NewDecoder(io.LimitReader(body, maxResponseBodySize)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode GitHub repository response: %w", err)
	}

	visibility := payload.Visibility
	if visibility == "" {
		visibility = "public"
		if payload.Private {
			visibility = "private"
		}
	}

	return &Repository{
		ID:         payload.ID,
		HTMLURL:    payload.HTMLURL,
		Visibility: visibility,
	}, nil
}

func decodeAPIError(response *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBodySize))
	if err != nil {
		return fmt.Errorf("read GitHub API error response: %w", err)
	}

	return &APIError{
		StatusCode: response.StatusCode,
		Body:       strings.TrimSpace(string(body)),
	}
}
