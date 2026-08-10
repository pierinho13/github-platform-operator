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
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	// DefaultDriftDetectionInterval is the periodic interval used to detect
	// changes made directly in GitHub when no Kubernetes event triggers a
	// reconciliation first.
	DefaultDriftDetectionInterval = 5 * time.Minute

	// MinDriftDetectionInterval prevents accidentally creating a tight polling
	// loop against the GitHub API.
	MinDriftDetectionInterval = time.Minute

	// MaxDriftDetectionInterval keeps drift detection bounded while still
	// allowing large installations to reduce GitHub API traffic substantially.
	MaxDriftDetectionInterval = 24 * time.Hour
)

// ValidateDriftDetectionInterval validates the operator-wide periodic GitHub
// drift-detection interval. Event-driven reconciliations for watched Kubernetes resources are unaffected.
func ValidateDriftDetectionInterval(interval time.Duration) error {
	if interval < MinDriftDetectionInterval || interval > MaxDriftDetectionInterval {
		return fmt.Errorf(
			"drift detection interval must be between %s and %s, got %s",
			MinDriftDetectionInterval,
			MaxDriftDetectionInterval,
			interval,
		)
	}
	return nil
}

func effectiveDriftDetectionInterval(interval time.Duration) time.Duration {
	if interval == 0 {
		return DefaultDriftDetectionInterval
	}
	return interval
}

func driftDetectionResult(interval time.Duration) ctrl.Result {
	return ctrl.Result{RequeueAfter: effectiveDriftDetectionInterval(interval)}
}
