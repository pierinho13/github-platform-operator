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
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRateLimitTransportMetricsContract(t *testing.T) {
	t.Parallel()

	registry := prometheus.NewRegistry()
	metricSet := newGitHubMetrics(registry)
	reset := time.Now().Add(10 * time.Minute).Unix()
	transport := &RateLimitTransport{
		Base: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					rateLimitHeaderLimit:     []string{"5000"},
					rateLimitHeaderRemaining: []string{"4242"},
					rateLimitHeaderReset:     []string{strconv.FormatInt(reset, 10)},
					rateLimitHeaderResource:  []string{rateLimitResourceCore},
				},
				Body:    io.NopCloser(strings.NewReader(`{"ok":true}`)),
				Request: request,
			}, nil
		}),
		gate:    &rateLimitGate{},
		metrics: metricSet,
	}

	request, err := http.NewRequest(http.MethodGet, "https://api.github.test/repos/example/repo", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	closeResponseBody(response.Body)

	if got := testutil.ToFloat64(metricSet.apiRequests.WithLabelValues(http.MethodGet, "200")); got != 1 {
		t.Fatalf("api request counter = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metricSet.rateLimitLimit.WithLabelValues(rateLimitResourceCore)); got != 5000 {
		t.Fatalf("rate-limit limit = %v, want 5000", got)
	}
	if got := testutil.ToFloat64(metricSet.rateLimitRemaining.WithLabelValues(rateLimitResourceCore)); got != 4242 {
		t.Fatalf("rate-limit remaining = %v, want 4242", got)
	}
	if got := testutil.ToFloat64(metricSet.rateLimitReset.WithLabelValues(rateLimitResourceCore)); got != float64(reset) {
		t.Fatalf("rate-limit reset = %v, want %d", got, reset)
	}
	if count := testutil.CollectAndCount(metricSet.apiRequestDuration); count != 1 {
		t.Fatalf("request duration metric families = %d, want 1", count)
	}
}

func TestRateLimitTransportRecordsTransportErrorsContract(t *testing.T) {
	t.Parallel()

	registry := prometheus.NewRegistry()
	metricSet := newGitHubMetrics(registry)
	transport := &RateLimitTransport{
		Base: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return nil, errors.New("network unavailable")
		}),
		gate:    &rateLimitGate{},
		metrics: metricSet,
	}

	request, err := http.NewRequest(http.MethodPost, "https://api.github.test/app/installations/1/access_tokens", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if _, err := transport.RoundTrip(request); err == nil {
		t.Fatal("expected transport error")
	}

	if got := testutil.ToFloat64(metricSet.apiTransportErrors.WithLabelValues(http.MethodPost)); got != 1 {
		t.Fatalf("transport error counter = %v, want 1", got)
	}
}

func TestRateLimitTransportRecordsRateLimitEventsContract(t *testing.T) {
	t.Parallel()

	registry := prometheus.NewRegistry()
	metricSet := newGitHubMetrics(registry)
	reset := time.Now().Add(2 * time.Minute).Unix()
	transport := &RateLimitTransport{
		Base: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Header: http.Header{
					rateLimitHeaderLimit:     []string{"5000"},
					rateLimitHeaderRemaining: []string{"0"},
					rateLimitHeaderReset:     []string{strconv.FormatInt(reset, 10)},
					rateLimitHeaderResource:  []string{rateLimitResourceCore},
				},
				Body:    io.NopCloser(strings.NewReader(`{"message":"API rate limit exceeded"}`)),
				Request: request,
			}, nil
		}),
		gate:    &rateLimitGate{},
		metrics: metricSet,
	}

	request, err := http.NewRequest(http.MethodGet, "https://api.github.test/orgs/example/teams", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	_, err = transport.RoundTrip(request)
	var rateErr *RateLimitError
	if !errors.As(err, &rateErr) {
		t.Fatalf("expected RateLimitError, got %v", err)
	}

	if got := testutil.ToFloat64(metricSet.rateLimitEvents.WithLabelValues("primary")); got != 1 {
		t.Fatalf("primary rate-limit events = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metricSet.rateLimitRemaining.WithLabelValues(rateLimitResourceCore)); got != 0 {
		t.Fatalf("rate-limit remaining = %v, want 0", got)
	}
	if got := testutil.ToFloat64(metricSet.rateLimitBlockedUntil); got < float64(reset) {
		t.Fatalf("blocked-until timestamp = %v, want at least %d", got, reset)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
