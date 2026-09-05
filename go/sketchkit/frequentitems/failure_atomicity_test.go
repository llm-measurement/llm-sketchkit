// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay Erramilli and Codex
package frequentitems

import (
	"bytes"
	"errors"
	"math"
	"testing"
)

func TestMutationPreflightFailureAtomicity(t *testing.T) {
	for _, operation := range []string{"negative weight", "add overflow", "merge overflow", "merge mismatch", "zero weight", "nil merge"} {
		t.Run(operation, func(t *testing.T) {
			receiver := newTestSketch(t, ProfileMicro)
			source := newTestSketch(t, ProfileMicro)
			mustAdd(t, source, 2, 1)
			weight := int64(1)
			var want error
			switch operation {
			case "negative weight":
				want = ErrNegativeWeight
			case "add overflow", "merge overflow":
				weight, want = math.MaxInt64, ErrWeightOverflow
			case "merge mismatch":
				source = newTestSketch(t, ProfileSmall)
				mustAdd(t, source, 2, 1)
				want = ErrIncompatibleMerge
			}
			mustAdd(t, receiver, 1, weight)
			before, err := receiver.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			sourceBefore, err := source.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			switch operation {
			case "negative weight":
				err = receiver.AddHash(3, -1)
			case "add overflow":
				err = receiver.AddHash(3, 1)
			case "zero weight":
				err = receiver.AddHash(3, 0)
			case "nil merge":
				err = receiver.Merge(nil)
			default:
				err = receiver.Merge(source)
			}
			if !errors.Is(err, want) {
				t.Fatalf("error = %v, want %v", err, want)
			}
			after, err := receiver.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			sourceAfter, err := source.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) || !bytes.Equal(sourceBefore, sourceAfter) {
				t.Fatal("receiver or distinct source changed")
			}
		})
	}
}
