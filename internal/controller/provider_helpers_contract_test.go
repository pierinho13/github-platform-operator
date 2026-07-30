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

package controller

import (
	"errors"
	"fmt"
	"testing"
	"time"

	githubclient "github.com/pierinho13/github-platform-operator/internal/github"
)

const contractWrappedErrorFormat = "wrapped: %w"

func TestDependencyFailureReasonContract(t *testing.T) {
	t.Parallel()

	if reason := dependencyFailureReason(fmt.Errorf(contractWrappedErrorFormat, errProviderSuspended)); reason != "ReconciliationSuspended" {
		t.Fatalf("unexpected suspended reason %q", reason)
	}
	if reason := dependencyFailureReason(errors.New("missing Secret")); reason != "DependencyUnavailable" {
		t.Fatalf("unexpected dependency reason %q", reason)
	}
}

func TestGitHubDeferredResultContract(t *testing.T) {
	t.Parallel()

	t.Run("uses GitHub rate-limit retry time", func(t *testing.T) {
		t.Parallel()

		retryAt := time.Now().Add(30 * time.Second)
		result, deferred := githubDeferredResult(fmt.Errorf(contractWrappedErrorFormat, &githubclient.RateLimitError{
			StatusCode: 429,
			RetryAt:    retryAt,
		}))
		if !deferred {
			t.Fatal("expected rate limit to be deferred")
		}
		if result.RequeueAfter < 28*time.Second || result.RequeueAfter > 31*time.Second {
			t.Fatalf("unexpected rate-limit requeue delay %s", result.RequeueAfter)
		}
	})

	t.Run("requeues suspended providers without returning an error", func(t *testing.T) {
		t.Parallel()

		result, deferred := githubDeferredResult(fmt.Errorf(contractWrappedErrorFormat, errProviderSuspended))
		if !deferred {
			t.Fatal("expected suspended provider to be deferred")
		}
		if result.RequeueAfter != 5*time.Minute {
			t.Fatalf("unexpected suspended-provider requeue delay %s", result.RequeueAfter)
		}
	})

	t.Run("does not defer ordinary reconciliation errors", func(t *testing.T) {
		t.Parallel()

		result, deferred := githubDeferredResult(errors.New("GitHub validation failed"))
		if deferred {
			t.Fatalf("ordinary errors must not be deferred: %#v", result)
		}
		if result.RequeueAfter != 0 {
			t.Fatalf("unexpected ordinary-error requeue delay %s", result.RequeueAfter)
		}
	})
}
