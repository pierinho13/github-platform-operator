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
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	controllermetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	metricsNamespace      = "github_platform_operator"
	metricsSubsystem      = "github"
	metricsMethodLabel    = "method"
	metricsResourceLabel  = "resource"
	rateLimitResourceCore = "core"
)

type githubMetrics struct {
	apiRequests           *prometheus.CounterVec
	apiRequestDuration    *prometheus.HistogramVec
	apiTransportErrors    *prometheus.CounterVec
	rateLimitLimit        *prometheus.GaugeVec
	rateLimitRemaining    *prometheus.GaugeVec
	rateLimitReset        *prometheus.GaugeVec
	rateLimitEvents       *prometheus.CounterVec
	rateLimitBlockedUntil prometheus.Gauge
}

func newGitHubMetrics(registerer prometheus.Registerer) *githubMetrics {
	m := &githubMetrics{
		apiRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "api_requests_total",
				Help:      "Total GitHub API responses observed by the shared HTTP client.",
			},
			[]string{metricsMethodLabel, "status_code"},
		),
		apiRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "api_request_duration_seconds",
				Help:      "GitHub API HTTP round-trip duration in seconds until response headers are received.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{metricsMethodLabel},
		),
		apiTransportErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "api_transport_errors_total",
				Help:      "Total GitHub API requests that failed before an HTTP response was received.",
			},
			[]string{metricsMethodLabel},
		),
		rateLimitLimit: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "rate_limit_limit",
				Help:      "Last observed GitHub API rate-limit ceiling for a rate-limit resource.",
			},
			[]string{metricsResourceLabel},
		),
		rateLimitRemaining: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "rate_limit_remaining",
				Help:      "Last observed GitHub API requests remaining for a rate-limit resource.",
			},
			[]string{metricsResourceLabel},
		),
		rateLimitReset: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "rate_limit_reset_timestamp_seconds",
				Help:      "Last observed GitHub API rate-limit reset time as a Unix timestamp.",
			},
			[]string{metricsResourceLabel},
		),
		rateLimitEvents: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "rate_limit_events_total",
				Help:      "Total GitHub API rate-limit responses detected by kind.",
			},
			[]string{"kind"},
		),
		rateLimitBlockedUntil: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "rate_limit_blocked_until_timestamp_seconds",
				Help:      "Unix timestamp until which the shared GitHub API gate is blocked; zero means no block has been observed.",
			},
		),
	}

	registerer.MustRegister(
		m.apiRequests,
		m.apiRequestDuration,
		m.apiTransportErrors,
		m.rateLimitLimit,
		m.rateLimitRemaining,
		m.rateLimitReset,
		m.rateLimitEvents,
		m.rateLimitBlockedUntil,
	)

	return m
}

var defaultGitHubMetrics = newGitHubMetrics(controllermetrics.Registry)

func (m *githubMetrics) observeResponse(request *http.Request, response *http.Response, duration time.Duration) {
	if m == nil || response == nil {
		return
	}

	method := normalizeHTTPMethod(request)
	m.apiRequests.WithLabelValues(method, strconv.Itoa(response.StatusCode)).Inc()
	m.apiRequestDuration.WithLabelValues(method).Observe(duration.Seconds())
	m.observeRateLimitHeaders(response.Header)
}

func (m *githubMetrics) observeTransportError(request *http.Request, duration time.Duration) {
	if m == nil {
		return
	}

	method := normalizeHTTPMethod(request)
	m.apiTransportErrors.WithLabelValues(method).Inc()
	m.apiRequestDuration.WithLabelValues(method).Observe(duration.Seconds())
}

func (m *githubMetrics) observeRateLimitHeaders(header http.Header) {
	if m == nil || header == nil {
		return
	}

	resource := strings.TrimSpace(header.Get(rateLimitHeaderResource))
	if resource == "" {
		resource = rateLimitResourceCore
	}

	setGaugeFromHeader(m.rateLimitLimit.WithLabelValues(resource), header.Get(rateLimitHeaderLimit))
	setGaugeFromHeader(m.rateLimitRemaining.WithLabelValues(resource), header.Get(rateLimitHeaderRemaining))
	setGaugeFromHeader(m.rateLimitReset.WithLabelValues(resource), header.Get(rateLimitHeaderReset))
}

func (m *githubMetrics) observeRateLimit(
	response *http.Response,
	rateErr *RateLimitError,
	blockedUntil time.Time,
) {
	if m == nil || rateErr == nil {
		return
	}

	m.rateLimitEvents.WithLabelValues(rateLimitKind(response, rateErr)).Inc()
	if !blockedUntil.IsZero() {
		m.rateLimitBlockedUntil.Set(float64(blockedUntil.Unix()))
	}
}

func setGaugeFromHeader(gauge prometheus.Gauge, value string) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return
	}
	gauge.Set(parsed)
}

func normalizeHTTPMethod(request *http.Request) string {
	if request == nil || strings.TrimSpace(request.Method) == "" {
		return "UNKNOWN"
	}
	return strings.ToUpper(request.Method)
}

func rateLimitKind(response *http.Response, rateErr *RateLimitError) string {
	if response != nil && strings.TrimSpace(response.Header.Get(rateLimitHeaderRemaining)) == "0" {
		return "primary"
	}

	body := ""
	if rateErr != nil {
		body = strings.ToLower(rateErr.Body)
	}
	if response != nil && (response.StatusCode == http.StatusTooManyRequests ||
		strings.TrimSpace(response.Header.Get("Retry-After")) != "") {
		return "secondary"
	}
	if strings.Contains(body, "secondary rate limit") || strings.Contains(body, "abuse detection") {
		return "secondary"
	}
	return "unknown"
}
