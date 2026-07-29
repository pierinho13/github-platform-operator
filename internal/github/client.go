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
	"errors"
)

// ErrNotFound indicates that a GitHub repository does not exist.
var ErrNotFound = errors.New("github repository not found")

// Repository contains the GitHub fields required by the controller.
type Repository struct {
	ID         int64
	HTMLURL    string
	Visibility string
}

// RepositoryClient defines the GitHub repository operations used by the controller.
type RepositoryClient interface {
	GetRepository(ctx context.Context, organization, name string) (*Repository, error)
	CreateRepository(ctx context.Context, organization, name string, private bool) (*Repository, error)
	UpdateRepositoryVisibility(
		ctx context.Context,
		organization string,
		name string,
		visibility string,
	) (*Repository, error)
	DeleteRepository(ctx context.Context, organization, name string) error
}
