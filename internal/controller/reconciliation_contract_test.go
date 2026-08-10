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
	"testing"
	"time"
)

func TestDriftDetectionIntervalContract(t *testing.T) {
	t.Parallel()

	if got := effectiveDriftDetectionInterval(0); got != DefaultDriftDetectionInterval {
		t.Fatalf("zero interval resolved to %s, want %s", got, DefaultDriftDetectionInterval)
	}

	custom := 17 * time.Minute
	if got := effectiveDriftDetectionInterval(custom); got != custom {
		t.Fatalf("custom interval resolved to %s, want %s", got, custom)
	}

	valid := []time.Duration{
		MinDriftDetectionInterval,
		DefaultDriftDetectionInterval,
		MaxDriftDetectionInterval,
	}
	for _, interval := range valid {
		if err := ValidateDriftDetectionInterval(interval); err != nil {
			t.Errorf("expected %s to be valid: %v", interval, err)
		}
	}

	invalid := []time.Duration{
		MinDriftDetectionInterval - time.Second,
		MaxDriftDetectionInterval + time.Second,
	}
	for _, interval := range invalid {
		if err := ValidateDriftDetectionInterval(interval); err == nil {
			t.Errorf("expected %s to be rejected", interval)
		}
	}
}
