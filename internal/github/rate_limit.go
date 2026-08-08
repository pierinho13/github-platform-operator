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
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	fallbackSecondaryRateLimitDelay = time.Minute
	rateLimitHeaderLimit            = "X-Ratelimit-Limit"
	rateLimitHeaderRemaining        = "X-Ratelimit-Remaining"
	rateLimitHeaderReset            = "X-Ratelimit-Reset"
	rateLimitHeaderResource         = "X-Ratelimit-Resource"
)

// RateLimitError indicates that GitHub asked the operator to stop making
// requests temporarily. It can represent both primary and secondary limits.
type RateLimitError struct {
	StatusCode int
	Body       string
	RetryAt    time.Time
}

func (e *RateLimitError) Error() string {
	if e == nil {
		return "GitHub API rate limited"
	}
	if e.RetryAt.IsZero() {
		return fmt.Sprintf("GitHub API rate limited with status %d", e.StatusCode)
	}
	return fmt.Sprintf(
		"GitHub API rate limited with status %d; retry after %s",
		e.StatusCode,
		e.RetryAt.UTC().Format(time.RFC3339),
	)
}

// RetryAfter returns a positive controller-runtime requeue duration.
func (e *RateLimitError) RetryAfter() time.Duration {
	if e == nil || e.RetryAt.IsZero() {
		return fallbackSecondaryRateLimitDelay
	}
	delay := time.Until(e.RetryAt)
	if delay < time.Second {
		return time.Second
	}
	return delay
}

type rateLimitGate struct {
	mu           sync.Mutex
	blockedUntil time.Time
	statusCode   int
	body         string
}

func (g *rateLimitGate) currentError(now time.Time) *RateLimitError {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.blockedUntil.After(now) {
		return nil
	}
	return &RateLimitError{
		StatusCode: g.statusCode,
		Body:       g.body,
		RetryAt:    g.blockedUntil,
	}
}

func (g *rateLimitGate) block(until time.Time, statusCode int, body string) time.Time {
	g.mu.Lock()
	defer g.mu.Unlock()

	if until.After(g.blockedUntil) {
		g.blockedUntil = until
		g.statusCode = statusCode
		g.body = body
	}
	return g.blockedUntil
}

// RateLimitTransport adds a shared, reactive GitHub rate-limit gate.
//
// It deliberately does not impose a normal per-second throttle. Requests remain
// as fast as before until GitHub explicitly returns a primary or secondary
// rate-limit response. At that point all clients sharing this transport pause
// until Retry-After or X-RateLimit-Reset permits another request.
type RateLimitTransport struct {
	Base http.RoundTripper

	gateOnce sync.Once
	gate     *rateLimitGate
	metrics  *githubMetrics
}

// NewRateLimitedHTTPClient creates an HTTP client whose rate-limit state is
// shared by every REST client that uses this instance.
func NewRateLimitedHTTPClient() *http.Client {
	return &http.Client{
		Timeout: defaultRequestTimeout,
		Transport: &RateLimitTransport{
			Base:    http.DefaultTransport,
			gate:    &rateLimitGate{},
			metrics: defaultGitHubMetrics,
		},
	}
}

// RoundTrip implements http.RoundTripper.
func (t *RateLimitTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if t == nil {
		return nil, errors.New("GitHub rate limit transport is nil")
	}

	t.gateOnce.Do(func() {
		if t.gate == nil {
			t.gate = &rateLimitGate{}
		}
	})
	gate := t.gate
	if rateErr := gate.currentError(time.Now()); rateErr != nil {
		return nil, rateErr
	}

	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	requestStartedAt := time.Now()
	response, err := base.RoundTrip(request)
	duration := time.Since(requestStartedAt)
	metrics := t.metrics
	if metrics == nil {
		metrics = defaultGitHubMetrics
	}
	if err != nil {
		metrics.observeTransportError(request, duration)
		return nil, err
	}
	metrics.observeResponse(request, response, duration)

	rateErr, err := rateLimitErrorFromResponse(response)
	if err != nil {
		closeResponseBody(response.Body)
		return nil, err
	}
	if rateErr == nil {
		return response, nil
	}

	blockedUntil := gate.block(rateErr.RetryAt, rateErr.StatusCode, rateErr.Body)
	metrics.observeRateLimit(response, rateErr, blockedUntil)
	closeResponseBody(response.Body)
	return nil, rateErr
}

func rateLimitErrorFromResponse(response *http.Response) (*RateLimitError, error) {
	if response == nil {
		return nil, nil
	}
	if response.StatusCode != http.StatusForbidden &&
		response.StatusCode != http.StatusTooManyRequests {
		return nil, nil
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBodySize))
	if err != nil {
		return nil, fmt.Errorf("read GitHub rate limit response: %w", err)
	}
	response.Body = io.NopCloser(bytes.NewReader(body))

	bodyText := strings.TrimSpace(string(body))
	lowerBody := strings.ToLower(bodyText)
	remaining := strings.TrimSpace(response.Header.Get(rateLimitHeaderRemaining))
	retryAfter := strings.TrimSpace(response.Header.Get("Retry-After"))

	isRateLimited := response.StatusCode == http.StatusTooManyRequests ||
		remaining == "0" ||
		retryAfter != "" ||
		strings.Contains(lowerBody, "rate limit") ||
		strings.Contains(lowerBody, "abuse detection")

	if !isRateLimited {
		return nil, nil
	}

	now := time.Now()
	retryAt := now.Add(fallbackSecondaryRateLimitDelay)

	if value := retryAfter; value != "" {
		if seconds, parseErr := strconv.Atoi(value); parseErr == nil {
			retryAt = now.Add(time.Duration(seconds) * time.Second)
		} else if date, parseErr := http.ParseTime(value); parseErr == nil {
			retryAt = date
		}
	} else if remaining == "0" {
		if reset, parseErr := strconv.ParseInt(
			strings.TrimSpace(response.Header.Get(rateLimitHeaderReset)),
			10,
			64,
		); parseErr == nil && reset > 0 {
			retryAt = time.Unix(reset, 0)
		}
	}

	// Small safety margin prevents a request from arriving exactly at reset.
	retryAt = retryAt.Add(time.Second)

	return &RateLimitError{
		StatusCode: response.StatusCode,
		Body:       bodyText,
		RetryAt:    retryAt,
	}, nil
}
