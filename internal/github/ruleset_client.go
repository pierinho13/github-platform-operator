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
	"encoding/json"
)

// RulesetBypassActor is a GitHub ruleset bypass actor.
type RulesetBypassActor struct {
	ActorID    *int64 `json:"actor_id"`
	ActorType  string `json:"actor_type"`
	BypassMode string `json:"bypass_mode,omitempty"`
}

// RulesetRefNameCondition selects refs by include and exclude patterns.
type RulesetRefNameCondition struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

// RulesetConditions contains repository ruleset conditions.
type RulesetConditions struct {
	RefName *RulesetRefNameCondition `json:"ref_name,omitempty"`
}

// RulesetRule is a GitHub ruleset rule. Parameters remain raw JSON so newly
// introduced GitHub rule parameters do not require a client release.
type RulesetRule struct {
	Type       string          `json:"type"`
	Parameters json.RawMessage `json:"parameters,omitempty"`
}

// RepositoryRulesetUpsert contains the mutable ruleset fields.
type RepositoryRulesetUpsert struct {
	Name         string                `json:"name"`
	Target       string                `json:"target"`
	Enforcement  string                `json:"enforcement"`
	BypassActors *[]RulesetBypassActor `json:"bypass_actors,omitempty"`
	Conditions   *RulesetConditions    `json:"conditions,omitempty"`
	Rules        []RulesetRule         `json:"rules"`
}

// RepositoryRuleset contains the observed GitHub ruleset.
type RepositoryRuleset struct {
	ID           int64
	Name         string
	Target       string
	SourceType   string
	Source       string
	Enforcement  string
	BypassActors []RulesetBypassActor
	Conditions   *RulesetConditions
	Rules        []RulesetRule
	HTMLURL      string
}

// RepositoryRulesetSummary is returned by the list endpoint.
type RepositoryRulesetSummary struct {
	ID          int64
	Name        string
	SourceType  string
	Source      string
	Enforcement string
}

// RepositoryRulesetClientFactory creates clients for repository rulesets.
type RepositoryRulesetClientFactory interface {
	NewRepositoryRulesetClient(token, baseURL string) (RepositoryRulesetClient, error)
}

// NewRepositoryRulesetClient creates a REST-backed ruleset client.
func (f RESTClientFactory) NewRepositoryRulesetClient(
	token string,
	baseURL string,
) (RepositoryRulesetClient, error) {
	return NewRESTClientWithHTTPClient(token, baseURL, f.HTTPClient)
}

// RepositoryRulesetClient defines the GitHub ruleset operations used by the controller.
type RepositoryRulesetClient interface {
	GetRepository(ctx context.Context, organization, name string) (*Repository, error)
	ListRepositoryRulesets(
		ctx context.Context,
		organization string,
		repository string,
	) ([]RepositoryRulesetSummary, error)
	GetRepositoryRuleset(
		ctx context.Context,
		organization string,
		repository string,
		rulesetID int64,
	) (*RepositoryRuleset, error)
	CreateRepositoryRuleset(
		ctx context.Context,
		organization string,
		repository string,
		input RepositoryRulesetUpsert,
	) (*RepositoryRuleset, error)
	UpdateRepositoryRuleset(
		ctx context.Context,
		organization string,
		repository string,
		rulesetID int64,
		input RepositoryRulesetUpsert,
	) (*RepositoryRuleset, error)
	DeleteRepositoryRuleset(
		ctx context.Context,
		organization string,
		repository string,
		rulesetID int64,
	) error
}
