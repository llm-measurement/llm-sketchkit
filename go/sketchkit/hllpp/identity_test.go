// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

package hllpp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestEstimateIdentityVectors(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "..", "vectors", "identity", "hllpp_*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("estimate vectors: %v", err)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var vector struct {
			Schema    int    `json:"schema_version"`
			Generator string `json:"generator"`
			Cases     []struct {
				Name       string  `json:"name"`
				Profile    Profile `json:"profile"`
				Count      int     `json:"count"`
				Seed       uint64  `json:"seed"`
				ForceDense bool    `json:"force_dense"`
				Bits       string  `json:"estimate_bits"`
				WireSHA256 string  `json:"wire_sha256"`
				MaxULPs    uint64  `json:"max_ulps"`
			} `json:"cases"`
		}
		if err := json.Unmarshal(data, &vector); err != nil {
			t.Fatal(err)
		}
		if vector.Schema != 1 || vector.Generator != "splitmix64" || len(vector.Cases) == 0 {
			t.Fatal("invalid estimate vector header")
		}
		for _, c := range vector.Cases {
			t.Run(c.Name, func(t *testing.T) {
				s := newTestSketch(t, c.Profile)
				state := c.Seed
				for i := 0; i < c.Count; i++ {
					s.AddHash(splitmix64(state))
					state += 0x9e3779b97f4a7c15
				}
				if c.ForceDense {
					s.ForceDense()
				}
				wire, err := s.MarshalBinary()
				if err != nil {
					t.Fatal(err)
				}
				digest := sha256.Sum256(wire)
				if hex.EncodeToString(digest[:]) != c.WireSHA256 {
					t.Fatal("wire digest mismatch")
				}
				want, err := strconv.ParseUint(c.Bits, 16, 64)
				if err != nil {
					t.Fatal(err)
				}
				parsed, err := Parse(wire)
				if err != nil {
					t.Fatal(err)
				}
				for _, estimate := range []float64{s.Estimate(), parsed.Estimate()} {
					got := math.Float64bits(estimate)
					distance := got - want
					if got < want {
						distance = want - got
					}
					if math.IsNaN(estimate) || math.IsInf(estimate, 0) || estimate < 0 || distance > c.MaxULPs {
						t.Fatalf("estimate bits = %016x, want %s within %d ULPs", got, c.Bits, c.MaxULPs)
					}
				}
			})
		}
	}
}
