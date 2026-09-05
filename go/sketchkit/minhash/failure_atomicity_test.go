// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay Erramilli and Codex
package minhash

import (
	"bytes"
	"errors"
	"math"
	"testing"
)

func TestMutationFailureAtomicity(t *testing.T) {
	for _, operation := range []string{"add overflow", "merge overflow", "merge mismatch", "nil merge"} {
		t.Run(operation, func(t *testing.T) {
			receiver := newTestSketch(t, ProfileMicro)
			source := newTestSketch(t, ProfileMicro)
			mustAdd(t, receiver, 1)
			mustAdd(t, source, 2)
			var want error
			switch operation {
			case "add overflow", "merge overflow":
				// Exercise the uint64 boundary without iterating 2^64 updates.
				receiver.populatedCount = math.MaxUint64
				want = ErrCountOverflow
			case "merge mismatch":
				source = newTestSketch(t, ProfileSmall)
				mustAdd(t, source, 2)
				want = ErrIncompatibleMerge
			}
			before, err := receiver.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			sourceBefore, err := source.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			switch operation {
			case "add overflow":
				err = receiver.AddHash(3)
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
