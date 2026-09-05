// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay Erramilli and Codex
package hllpp

import (
	"bytes"
	"errors"
	"testing"

	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
)

func TestMergeFailureAtomicity(t *testing.T) {
	for _, representation := range []string{"sparse", "dense"} {
		for _, operation := range []string{"precision mismatch", "domain mismatch", "nil merge"} {
			t.Run(operation+"/"+representation, func(t *testing.T) {
				receiver := newTestSketch(t, ProfileMicro)
				receiver.AddHash(1)
				if representation == "dense" {
					receiver.ForceDense()
				}
				profile, domain := ProfileMicro, sketchhash.PromptV1
				var want error
				switch operation {
				case "precision mismatch":
					profile, want = ProfileSmall, ErrPrecisionMismatch
				case "domain mismatch":
					domain, want = sketchhash.UserV1, ErrIncompatibleMerge
				}
				source, err := New(profile, domain, sketchhash.HMACSHA25664)
				if err != nil {
					t.Fatal(err)
				}
				source.AddHash(2)
				before, err := receiver.MarshalBinary()
				if err != nil {
					t.Fatal(err)
				}
				sourceBefore, err := source.MarshalBinary()
				if err != nil {
					t.Fatal(err)
				}
				if operation == "nil merge" {
					err = receiver.Merge(nil)
				} else {
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
}
