//go:build !race

// Code authors: Vijay Erramilli and Codex
package hllpp

import "testing"

func TestAccuracyLargeCardinality(t *testing.T) {
	t.Parallel()

	const cardinality = 10_000_000
	profiles := []Profile{ProfileMicro, ProfileSmall}

	for _, profile := range profiles {
		profile := profile
		t.Run(string(profile), func(t *testing.T) {
			t.Parallel()

			for _, result := range collectAccuracyResults(t, profile, []int{cardinality}, 10) {
				assertAccuracyResultWithinBound(t, result)
				t.Log(result.LogLine())
			}
		})
	}
}
