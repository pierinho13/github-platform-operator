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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"time"
)

// GetEnvironment returns a repository deployment environment.
func (c *RESTClient) GetEnvironment(
	ctx context.Context,
	organization string,
	repository string,
	environment string,
) (*Environment, error) {
	response, err := c.do(ctx, http.MethodGet, c.environmentEndpoint(
		organization, repository, environment,
	), nil)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(response.Body)

	if response.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, decodeAPIError(response)
	}
	return decodeEnvironment(response.Body)
}

// UpsertEnvironment creates an environment or leaves its advanced protection settings untouched.
func (c *RESTClient) UpsertEnvironment(
	ctx context.Context,
	organization string,
	repository string,
	environment string,
) (*Environment, error) {
	response, err := c.do(ctx, http.MethodPut, c.environmentEndpoint(
		organization, repository, environment,
	), bytes.NewReader([]byte(`{}`)))
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(response.Body)

	if response.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, decodeAPIError(response)
	}
	return decodeEnvironment(response.Body)
}

// DeleteEnvironment deletes a repository deployment environment.
func (c *RESTClient) DeleteEnvironment(
	ctx context.Context,
	organization string,
	repository string,
	environment string,
) error {
	response, err := c.do(ctx, http.MethodDelete, c.environmentEndpoint(
		organization, repository, environment,
	), nil)
	if err != nil {
		return err
	}
	defer closeResponseBody(response.Body)

	if response.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return decodeAPIError(response)
	}
	return nil
}

// GetActionsPublicKey returns the public key for the selected Actions target.
func (c *RESTClient) GetActionsPublicKey(
	ctx context.Context,
	target ActionsTarget,
) (*ActionsPublicKey, error) {
	endpoint, err := c.actionsSecretPublicKeyEndpoint(target)
	if err != nil {
		return nil, err
	}
	response, err := c.do(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(response.Body)

	if response.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, decodeAPIError(response)
	}

	var payload struct {
		KeyID string `json:"key_id"`
		Key   string `json:"key"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxResponseBodySize)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode GitHub Actions public key: %w", err)
	}
	return &ActionsPublicKey{KeyID: payload.KeyID, Key: payload.Key}, nil
}

// GetActionsSecret returns metadata for a secret without exposing its value.
func (c *RESTClient) GetActionsSecret(
	ctx context.Context,
	target ActionsTarget,
	name string,
) (*ActionsSecret, error) {
	endpoint, err := c.actionsSecretEndpoint(target, name)
	if err != nil {
		return nil, err
	}
	response, err := c.do(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(response.Body)

	if response.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, decodeAPIError(response)
	}

	secret, err := decodeActionsSecret(response.Body)
	if err != nil {
		return nil, err
	}
	if target.Scope == ActionsTargetScopeOrganization && secret.Visibility == "selected" {
		secret.SelectedRepositoryIDs, err = c.getSelectedRepositoryIDs(ctx, endpoint+"/repositories")
		if err != nil {
			return nil, err
		}
	}
	return secret, nil
}

// UpsertActionsSecret creates or updates a secret.
func (c *RESTClient) UpsertActionsSecret(
	ctx context.Context,
	target ActionsTarget,
	name string,
	input ActionsSecretUpsert,
) error {
	endpoint, err := c.actionsSecretEndpoint(target, name)
	if err != nil {
		return err
	}
	requestBody, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode GitHub Actions secret request: %w", err)
	}
	response, err := c.do(ctx, http.MethodPut, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return err
	}
	defer closeResponseBody(response.Body)

	if response.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return decodeAPIError(response)
	}
	return nil
}

// DeleteActionsSecret deletes a secret.
func (c *RESTClient) DeleteActionsSecret(
	ctx context.Context,
	target ActionsTarget,
	name string,
) error {
	endpoint, err := c.actionsSecretEndpoint(target, name)
	if err != nil {
		return err
	}
	response, err := c.do(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	defer closeResponseBody(response.Body)

	if response.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return decodeAPIError(response)
	}
	return nil
}

// GetActionsVariable returns a variable and its visible value.
func (c *RESTClient) GetActionsVariable(
	ctx context.Context,
	target ActionsTarget,
	name string,
) (*ActionsVariable, error) {
	endpoint, err := c.actionsVariableEndpoint(target, name)
	if err != nil {
		return nil, err
	}
	response, err := c.do(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(response.Body)

	if response.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, decodeAPIError(response)
	}

	variable, err := decodeActionsVariable(response.Body)
	if err != nil {
		return nil, err
	}
	if target.Scope == ActionsTargetScopeOrganization && variable.Visibility == "selected" {
		variable.SelectedRepositoryIDs, err = c.getSelectedRepositoryIDs(ctx, endpoint+"/repositories")
		if err != nil {
			return nil, err
		}
	}
	return variable, nil
}

// CreateActionsVariable creates a variable.
func (c *RESTClient) CreateActionsVariable(
	ctx context.Context,
	target ActionsTarget,
	input ActionsVariableUpsert,
) error {
	endpoint, err := c.actionsVariableCollectionEndpoint(target)
	if err != nil {
		return err
	}
	return c.writeActionsVariable(ctx, http.MethodPost, endpoint, input)
}

// UpdateActionsVariable updates a variable.
func (c *RESTClient) UpdateActionsVariable(
	ctx context.Context,
	target ActionsTarget,
	name string,
	input ActionsVariableUpsert,
) error {
	endpoint, err := c.actionsVariableEndpoint(target, name)
	if err != nil {
		return err
	}
	return c.writeActionsVariable(ctx, http.MethodPatch, endpoint, input)
}

func (c *RESTClient) writeActionsVariable(
	ctx context.Context,
	method string,
	endpoint string,
	input ActionsVariableUpsert,
) error {
	requestBody, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode GitHub Actions variable request: %w", err)
	}
	response, err := c.do(ctx, method, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return err
	}
	defer closeResponseBody(response.Body)

	if response.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return decodeAPIError(response)
	}
	return nil
}

// DeleteActionsVariable deletes a variable.
func (c *RESTClient) DeleteActionsVariable(
	ctx context.Context,
	target ActionsTarget,
	name string,
) error {
	endpoint, err := c.actionsVariableEndpoint(target, name)
	if err != nil {
		return err
	}
	response, err := c.do(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	defer closeResponseBody(response.Body)

	if response.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return decodeAPIError(response)
	}
	return nil
}

func (c *RESTClient) environmentEndpoint(organization, repository, environment string) string {
	return fmt.Sprintf(
		"%s/repos/%s/%s/environments/%s",
		c.baseURL,
		url.PathEscape(organization),
		url.PathEscape(repository),
		url.PathEscape(environment),
	)
}

func (c *RESTClient) actionsSecretPublicKeyEndpoint(target ActionsTarget) (string, error) {
	switch target.Scope {
	case ActionsTargetScopeRepository:
		return fmt.Sprintf("%s/repos/%s/%s/actions/secrets/public-key", c.baseURL,
			url.PathEscape(target.Organization), url.PathEscape(target.Repository)), nil
	case ActionsTargetScopeEnvironment:
		return fmt.Sprintf("%s/repos/%s/%s/environments/%s/secrets/public-key", c.baseURL,
			url.PathEscape(target.Organization), url.PathEscape(target.Repository),
			url.PathEscape(target.Environment)), nil
	case ActionsTargetScopeOrganization:
		return fmt.Sprintf("%s/orgs/%s/actions/secrets/public-key", c.baseURL,
			url.PathEscape(target.Organization)), nil
	default:
		return "", fmt.Errorf("unsupported GitHub Actions target scope %q", target.Scope)
	}
}

func (c *RESTClient) actionsSecretEndpoint(target ActionsTarget, name string) (string, error) {
	switch target.Scope {
	case ActionsTargetScopeRepository:
		return fmt.Sprintf("%s/repos/%s/%s/actions/secrets/%s", c.baseURL,
			url.PathEscape(target.Organization), url.PathEscape(target.Repository), url.PathEscape(name)), nil
	case ActionsTargetScopeEnvironment:
		return fmt.Sprintf("%s/repos/%s/%s/environments/%s/secrets/%s", c.baseURL,
			url.PathEscape(target.Organization), url.PathEscape(target.Repository),
			url.PathEscape(target.Environment), url.PathEscape(name)), nil
	case ActionsTargetScopeOrganization:
		return fmt.Sprintf("%s/orgs/%s/actions/secrets/%s", c.baseURL,
			url.PathEscape(target.Organization), url.PathEscape(name)), nil
	default:
		return "", fmt.Errorf("unsupported GitHub Actions target scope %q", target.Scope)
	}
}

func (c *RESTClient) actionsVariableCollectionEndpoint(target ActionsTarget) (string, error) {
	switch target.Scope {
	case ActionsTargetScopeRepository:
		return fmt.Sprintf("%s/repos/%s/%s/actions/variables", c.baseURL,
			url.PathEscape(target.Organization), url.PathEscape(target.Repository)), nil
	case ActionsTargetScopeEnvironment:
		return fmt.Sprintf("%s/repos/%s/%s/environments/%s/variables", c.baseURL,
			url.PathEscape(target.Organization), url.PathEscape(target.Repository),
			url.PathEscape(target.Environment)), nil
	case ActionsTargetScopeOrganization:
		return fmt.Sprintf("%s/orgs/%s/actions/variables", c.baseURL,
			url.PathEscape(target.Organization)), nil
	default:
		return "", fmt.Errorf("unsupported GitHub Actions target scope %q", target.Scope)
	}
}

func (c *RESTClient) actionsVariableEndpoint(target ActionsTarget, name string) (string, error) {
	collection, err := c.actionsVariableCollectionEndpoint(target)
	if err != nil {
		return "", err
	}
	return collection + "/" + url.PathEscape(name), nil
}

func (c *RESTClient) getSelectedRepositoryIDs(ctx context.Context, endpoint string) ([]int64, error) {
	response, err := c.do(ctx, http.MethodGet, endpoint+"?per_page=100", nil)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(response.Body)

	if response.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, decodeAPIError(response)
	}
	var payload struct {
		Repositories []struct {
			ID int64 `json:"id"`
		} `json:"repositories"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxResponseBodySize)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode selected repositories response: %w", err)
	}
	ids := make([]int64, 0, len(payload.Repositories))
	for i := range payload.Repositories {
		ids = append(ids, payload.Repositories[i].ID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func decodeEnvironment(body io.Reader) (*Environment, error) {
	var payload struct {
		ID        int64     `json:"id"`
		Name      string    `json:"name"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	if err := json.NewDecoder(io.LimitReader(body, maxResponseBodySize)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode GitHub environment response: %w", err)
	}
	return &Environment{
		ID: payload.ID, Name: payload.Name, CreatedAt: payload.CreatedAt, UpdatedAt: payload.UpdatedAt,
	}, nil
}

func decodeActionsSecret(body io.Reader) (*ActionsSecret, error) {
	var payload struct {
		Name       string    `json:"name"`
		Visibility string    `json:"visibility"`
		CreatedAt  time.Time `json:"created_at"`
		UpdatedAt  time.Time `json:"updated_at"`
	}
	if err := json.NewDecoder(io.LimitReader(body, maxResponseBodySize)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode GitHub Actions secret response: %w", err)
	}
	return &ActionsSecret{
		Name: payload.Name, Visibility: payload.Visibility,
		CreatedAt: payload.CreatedAt, UpdatedAt: payload.UpdatedAt,
	}, nil
}

func decodeActionsVariable(body io.Reader) (*ActionsVariable, error) {
	var payload struct {
		Name       string    `json:"name"`
		Value      string    `json:"value"`
		Visibility string    `json:"visibility"`
		CreatedAt  time.Time `json:"created_at"`
		UpdatedAt  time.Time `json:"updated_at"`
	}
	if err := json.NewDecoder(io.LimitReader(body, maxResponseBodySize)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode GitHub Actions variable response: %w", err)
	}
	return &ActionsVariable{
		Name: payload.Name, Value: payload.Value, Visibility: payload.Visibility,
		CreatedAt: payload.CreatedAt, UpdatedAt: payload.UpdatedAt,
	}, nil
}
