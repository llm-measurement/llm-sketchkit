// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

package summary

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestIntervalVectors(t *testing.T) {
	data, err := os.ReadFile("../../../vectors/validation/summary_intervals.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors struct {
		Cases []struct {
			Name            string
			Repeat          int
			Start, End      int64
			Count           uint64
			Sketches, Error bool
			Suffix          string
		}
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	for _, v := range vectors.Cases {
		t.Run(v.Name, func(t *testing.T) {
			docs := []Envelope{}
			for i := 0; i < v.Repeat; i++ {
				e := fixture(t, "a", fmt.Sprintf("e%d%s", i, v.Suffix), 1, v.Count)
				e.ObservedStart, e.ObservedEnd = v.Start, v.End
				if !v.Sketches {
					e.Sketches = map[string]Payload{}
				}
				docs = append(docs, e)
			}
			if v.Suffix != "" {
				e := fixture(t, "a", "e0m", 1, 2)
				e.ObservedStart = 90
				e.Sketches = map[string]Payload{}
				docs = append(docs, e)
			}
			_, err := Combine(docs, []string{"a"})
			if (err != nil) != v.Error {
				t.Fatalf("error = %v, expected rejection %v", err, v.Error)
			}
		})
	}
}
