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
)

// GetTeamIDBySlug resolves an organization team slug to GitHub's numeric ID.
func (c *RESTClient) GetTeamIDBySlug(
	ctx context.Context,
	organization string,
	teamSlug string,
) (int64, error) {
	endpoint := fmt.Sprintf(
		"%s/orgs/%s/teams/%s",
		c.baseURL,
		url.PathEscape(organization),
		url.PathEscape(teamSlug),
	)
	return c.getRulesetActorID(ctx, endpoint, "team")
}

// GetUserIDByUsername resolves a GitHub username to its numeric user ID.
func (c *RESTClient) GetUserIDByUsername(
	ctx context.Context,
	username string,
) (int64, error) {
	endpoint := fmt.Sprintf("%s/users/%s", c.baseURL, url.PathEscape(username))
	return c.getRulesetActorID(ctx, endpoint, "user")
}

func (c *RESTClient) getRulesetActorID(
	ctx context.Context,
	endpoint string,
	actorType string,
) (int64, error) {
	response, err := c.do(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	defer closeResponseBody(response.Body)

	if response.StatusCode == http.StatusNotFound {
		return 0, ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return 0, decodeAPIError(response)
	}

	var payload struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxResponseBodySize)).Decode(&payload); err != nil {
		return 0, fmt.Errorf("decode GitHub %s response: %w", actorType, err)
	}
	if payload.ID <= 0 {
		return 0, fmt.Errorf("GitHub %s response contains an invalid ID %d", actorType, payload.ID)
	}

	return payload.ID, nil
}

// ListRepositoryRulesets returns only repository-owned rulesets.
// Organization and enterprise parent rulesets are intentionally excluded.
func (c *RESTClient) ListRepositoryRulesets(
	ctx context.Context,
	organization string,
	repository string,
) ([]RepositoryRulesetSummary, error) {
	var result []RepositoryRulesetSummary

	for page := 1; ; page++ {
		endpoint := fmt.Sprintf(
			"%s?includes_parents=false&per_page=100&page=%d",
			c.repositoryRulesetsEndpoint(organization, repository),
			page,
		)
		response, err := c.do(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}

		if response.StatusCode == http.StatusNotFound {
			closeResponseBody(response.Body)
			return nil, ErrNotFound
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			apiErr := decodeAPIError(response)
			closeResponseBody(response.Body)
			return nil, apiErr
		}

		var payload []struct {
			ID          int64  `json:"id"`
			Name        string `json:"name"`
			SourceType  string `json:"source_type"`
			Source      string `json:"source"`
			Enforcement string `json:"enforcement"`
		}
		decodeErr := json.NewDecoder(
			io.LimitReader(response.Body, maxResponseBodySize),
		).Decode(&payload)
		closeResponseBody(response.Body)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode repository rulesets response: %w", decodeErr)
		}

		for i := range payload {
			result = append(result, RepositoryRulesetSummary{
				ID:          payload[i].ID,
				Name:        payload[i].Name,
				SourceType:  payload[i].SourceType,
				Source:      payload[i].Source,
				Enforcement: payload[i].Enforcement,
			})
		}

		if len(payload) < 100 {
			return result, nil
		}
	}
}

// GetRepositoryRuleset returns one repository ruleset.
func (c *RESTClient) GetRepositoryRuleset(
	ctx context.Context,
	organization string,
	repository string,
	rulesetID int64,
) (*RepositoryRuleset, error) {
	response, err := c.do(
		ctx,
		http.MethodGet,
		c.repositoryRulesetEndpoint(organization, repository, rulesetID),
		nil,
	)
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

	return decodeRepositoryRuleset(response.Body)
}

// CreateRepositoryRuleset creates a repository ruleset.
func (c *RESTClient) CreateRepositoryRuleset(
	ctx context.Context,
	organization string,
	repository string,
	input RepositoryRulesetUpsert,
) (*RepositoryRuleset, error) {
	requestBody, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode create repository ruleset request: %w", err)
	}

	response, err := c.do(
		ctx,
		http.MethodPost,
		c.repositoryRulesetsEndpoint(organization, repository),
		bytes.NewReader(requestBody),
	)
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

	return decodeRepositoryRuleset(response.Body)
}

// UpdateRepositoryRuleset replaces all mutable ruleset fields.
func (c *RESTClient) UpdateRepositoryRuleset(
	ctx context.Context,
	organization string,
	repository string,
	rulesetID int64,
	input RepositoryRulesetUpsert,
) (*RepositoryRuleset, error) {
	requestBody, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode update repository ruleset request: %w", err)
	}

	response, err := c.do(
		ctx,
		http.MethodPut,
		c.repositoryRulesetEndpoint(organization, repository, rulesetID),
		bytes.NewReader(requestBody),
	)
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

	return decodeRepositoryRuleset(response.Body)
}

// DeleteRepositoryRuleset deletes a repository ruleset.
func (c *RESTClient) DeleteRepositoryRuleset(
	ctx context.Context,
	organization string,
	repository string,
	rulesetID int64,
) error {
	response, err := c.do(
		ctx,
		http.MethodDelete,
		c.repositoryRulesetEndpoint(organization, repository, rulesetID),
		nil,
	)
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

func (c *RESTClient) repositoryRulesetsEndpoint(
	organization string,
	repository string,
) string {
	return fmt.Sprintf(
		"%s/repos/%s/%s/rulesets",
		c.baseURL,
		url.PathEscape(organization),
		url.PathEscape(repository),
	)
}

func (c *RESTClient) repositoryRulesetEndpoint(
	organization string,
	repository string,
	rulesetID int64,
) string {
	return fmt.Sprintf(
		"%s/%d",
		c.repositoryRulesetsEndpoint(organization, repository),
		rulesetID,
	)
}

func decodeRepositoryRuleset(body io.Reader) (*RepositoryRuleset, error) {
	var payload struct {
		ID           int64                `json:"id"`
		Name         string               `json:"name"`
		Target       string               `json:"target"`
		SourceType   string               `json:"source_type"`
		Source       string               `json:"source"`
		Enforcement  string               `json:"enforcement"`
		BypassActors []RulesetBypassActor `json:"bypass_actors"`
		Conditions   *RulesetConditions   `json:"conditions"`
		Rules        []RulesetRule        `json:"rules"`
		Links        struct {
			HTML struct {
				Href string `json:"href"`
			} `json:"html"`
		} `json:"_links"`
	}
	if err := json.NewDecoder(io.LimitReader(body, maxResponseBodySize)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode repository ruleset response: %w", err)
	}

	return &RepositoryRuleset{
		ID:           payload.ID,
		Name:         payload.Name,
		Target:       payload.Target,
		SourceType:   payload.SourceType,
		Source:       payload.Source,
		Enforcement:  payload.Enforcement,
		BypassActors: payload.BypassActors,
		Conditions:   payload.Conditions,
		Rules:        payload.Rules,
		HTMLURL:      payload.Links.HTML.Href,
	}, nil
}
