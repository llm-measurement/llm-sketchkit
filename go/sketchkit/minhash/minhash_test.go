// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

package minhash

import (
	"errors"
	"math"
	"path/filepath"
	"sort"
	"testing"

	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
	sketchpb "github.com/llm-measurement/llm-sketchkit/go/sketchkit/internal/pb"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/internal/sketchtest"
	"google.golang.org/protobuf/proto"
)

type sketchVector struct {
	Name     string `json:"name"`
	Metadata struct {
		Kind                   string `json:"kind"`
		Profile                string `json:"profile"`
		HashDomain             string `json:"hash_domain"`
		HashAlgo               string `json:"hash_algo"`
		RepresentationMode     string `json:"representation_mode"`
		MinhashSignatureLength int    `json:"minhash_signature_length"`
	} `json:"metadata"`
	Operations []struct {
		Op      string `json:"op"`
		HashHex string `json:"hash_hex"`
		Source  string `json:"source"`
	} `json:"operations"`
	Expected struct {
		SerializedHex string `json:"serialized_hex"`
		Body          struct {
			PopulatedCount uint64   `json:"populated_count"`
			SignatureHex   []string `json:"signature_hex"`
			Jaccard        float64  `json:"jaccard"`
		} `json:"body"`
	} `json:"expected"`
}

func TestSketchVectors(t *testing.T) {
	t.Parallel()

	paths, err := filepath.Glob(filepath.Join("..", "..", "..", "vectors", "sketches", "minhash_*.json"))
	if err != nil {
		t.Fatalf("glob sketch vectors: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no MinHash vectors found")
	}

	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()

			vector := sketchtest.ReadJSON[sketchVector](t, path)
			got, bySource := buildVectorSketch(t, vector)
			assertVectorExpected(t, got, bySource, vector)
			sketchtest.AssertStable(t, got, Parse)
			sketchtest.AssertSerializedHex(t, got, vector.Expected.SerializedHex, Parse)
		})
	}
}

func TestMergeEqualsSignatureOfUnion(t *testing.T) {
	t.Parallel()

	left := newTestSketch(t, ProfileSmall)
	right := newTestSketch(t, ProfileSmall)
	union := newTestSketch(t, ProfileSmall)
	for i := uint64(0); i < 500; i++ {
		hash := splitmix64(0x11110000 + i)
		if i%3 == 0 {
			mustAdd(t, left, hash)
		} else {
			mustAdd(t, right, hash)
		}
		mustAdd(t, union, hash)
	}

	merged := left.Clone()
	if err := merged.Merge(right); err != nil {
		t.Fatalf("Merge(): %v", err)
	}
	assertSignatureEqual(t, merged.signature, union.signature)
}

func TestJaccardAccuracyProfiles(t *testing.T) {
	t.Parallel()

	scenarios := []struct {
		profile Profile
		meanMax float64
		p95Max  float64
	}{
		{profile: ProfileMicro, meanMax: 0.057, p95Max: 0.127},
		{profile: ProfileSmall, meanMax: 0.040, p95Max: 0.090},
		{profile: ProfileDefault, meanMax: 0.040, p95Max: 0.090},
		{profile: ProfileK256, meanMax: 0.030, p95Max: 0.070},
	}
	for _, scenario := range scenarios {
		scenario := scenario
		t.Run(string(scenario.profile), func(t *testing.T) {
			t.Parallel()

			mean, p95 := characterizeJaccardAccuracy(t, scenario.profile, 1000)
			if mean > scenario.meanMax || p95 > scenario.p95Max {
				t.Fatalf("profile=%s mean_abs_error=%.8f p95_abs_error=%.8f, want mean<=%.8f p95<=%.8f",
					scenario.profile, mean, p95, scenario.meanMax, scenario.p95Max)
			}
			t.Logf("minhash accuracy profile=%s k=%d pairs=%d mean_abs_error=%.8f p95_abs_error=%.8f mean_bound=%.8f p95_bound=%.8f",
				scenario.profile, profileLengths[scenario.profile], 1000, mean, p95, scenario.meanMax, scenario.p95Max)
		})
	}
}

func TestParseRejectsNonProfileSignatureLength(t *testing.T) {
	t.Parallel()

	sketch := newTestSketch(t, ProfileSmall)
	encoded, err := sketch.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary(): %v", err)
	}

	var message sketchpb.Sketch
	if err := proto.Unmarshal(encoded, &message); err != nil {
		t.Fatalf("unmarshal sketch: %v", err)
	}
	wrongLength := uint32(sketch.SignatureLength() - 1)
	message.GetMetadata().MinhashSignatureLength = &wrongLength

	mutated, err := proto.MarshalOptions{Deterministic: true}.Marshal(&message)
	if err != nil {
		t.Fatalf("marshal mutated sketch: %v", err)
	}
	_, err = Parse(mutated)
	if !errors.Is(err, ErrInvalidSignatureLength) {
		t.Fatalf("Parse() error = %v, want %v", err, ErrInvalidSignatureLength)
	}
}

func characterizeJaccardAccuracy(t *testing.T, profile Profile, pairCount int) (float64, float64) {
	t.Helper()

	errors := make([]float64, 0, pairCount)
	for trial := 0; trial < pairCount; trial++ {
		left := newTestSketch(t, profile)
		right := newTestSketch(t, profile)
		exact := addJaccardPair(t, left, right, uint64(trial), 256)
		estimate, err := left.JaccardEstimate(right)
		if err != nil {
			t.Fatalf("JaccardEstimate(): %v", err)
		}
		errors = append(errors, math.Abs(estimate-exact))
	}
	sort.Float64s(errors)
	sum := 0.0
	for _, value := range errors {
		sum += value
	}
	p95Index := int(math.Ceil(0.95*float64(len(errors)))) - 1

	return sum / float64(len(errors)), errors[p95Index]
}

func addJaccardPair(t *testing.T, left *Sketch, right *Sketch, trialSeed uint64, unionSize int) float64 {
	t.Helper()

	desired := 0.05 + float64((trialSeed*37)%91)/100.0
	intersection := int(math.Round(desired * float64(unionSize)))
	if intersection < 1 {
		intersection = 1
	}
	if intersection > unionSize-2 {
		intersection = unionSize - 2
	}
	remaining := unionSize - intersection
	leftOnly := remaining / 2
	rightOnly := remaining - leftOnly
	seed := 0x5510000000000000 ^ (trialSeed << 20)
	offset := uint64(0)
	for i := 0; i < intersection; i++ {
		hash := splitmix64(seed + offset)
		mustAdd(t, left, hash)
		mustAdd(t, right, hash)
		offset++
	}
	for i := 0; i < leftOnly; i++ {
		mustAdd(t, left, splitmix64(seed+offset))
		offset++
	}
	for i := 0; i < rightOnly; i++ {
		mustAdd(t, right, splitmix64(seed+offset))
		offset++
	}

	return float64(intersection) / float64(unionSize)
}

func buildVectorSketch(t *testing.T, vector sketchVector) (*Sketch, map[string]*Sketch) {
	t.Helper()

	if vector.Metadata.Kind != "MINHASH" {
		t.Fatalf("unsupported vector kind %q", vector.Metadata.Kind)
	}
	if vector.Metadata.HashAlgo != string(sketchhash.HMACSHA25664) {
		t.Fatalf("unsupported hash algorithm %q", vector.Metadata.HashAlgo)
	}
	profile := Profile(vector.Metadata.Profile)
	if vector.Metadata.MinhashSignatureLength != profileLengths[profile] {
		t.Fatalf("vector signature length = %d, want %d",
			vector.Metadata.MinhashSignatureLength, profileLengths[profile])
	}

	bySource := map[string]*Sketch{}
	sourceNames := make([]string, 0)
	for _, operation := range vector.Operations {
		if operation.Op != "add_hash" {
			t.Fatalf("unsupported operation %q", operation.Op)
		}
		source := operation.Source
		if source == "" {
			source = "default"
		}
		if bySource[source] == nil {
			bySource[source] = newTestSketch(t, profile)
			sourceNames = append(sourceNames, source)
		}
		mustAdd(t, bySource[source], sketchtest.ParseHashHex(t, operation.HashHex))
	}
	if len(bySource) == 0 {
		return newTestSketch(t, profile), bySource
	}

	sort.Strings(sourceNames)
	merged := bySource[sourceNames[0]].Clone()
	for _, source := range sourceNames[1:] {
		if err := merged.Merge(bySource[source]); err != nil {
			t.Fatalf("merge source %s: %v", source, err)
		}
	}

	return merged, bySource
}

func assertVectorExpected(t *testing.T, got *Sketch, bySource map[string]*Sketch, vector sketchVector) {
	t.Helper()

	if got.PopulatedCount() != vector.Expected.Body.PopulatedCount {
		t.Fatalf("populated count = %d, want %d", got.PopulatedCount(), vector.Expected.Body.PopulatedCount)
	}
	wantSignature := parseSignatureHex(t, vector.Expected.Body.SignatureHex)
	assertSignatureEqual(t, got.signature, wantSignature)
	if len(bySource) == 2 && vector.Expected.Body.Jaccard != 0 {
		left := bySource["left"]
		right := bySource["right"]
		if left == nil || right == nil {
			t.Fatal("jaccard vector must use left and right sources")
		}
		estimate, err := left.JaccardEstimate(right)
		if err != nil {
			t.Fatalf("JaccardEstimate(): %v", err)
		}
		if math.Abs(estimate-vector.Expected.Body.Jaccard) > 1e-15 {
			t.Fatalf("jaccard estimate = %.18f, want %.18f", estimate, vector.Expected.Body.Jaccard)
		}
	}
}

func assertSignatureEqual(t *testing.T, got []uint64, want []uint64) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("signature length = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("signature[%d] = %016x, want %016x", i, got[i], want[i])
		}
	}
}

func parseSignatureHex(t *testing.T, values []string) []uint64 {
	t.Helper()

	out := make([]uint64, 0, len(values))
	for _, value := range values {
		out = append(out, sketchtest.ParseHashHex(t, value))
	}

	return out
}

func newTestSketch(t *testing.T, profile Profile) *Sketch {
	t.Helper()

	sketch, err := New(profile, sketchhash.PromptV1, sketchhash.HMACSHA25664)
	if err != nil {
		t.Fatalf("New(%s): %v", profile, err)
	}

	return sketch
}

func mustAdd(t *testing.T, sketch *Sketch, hash uint64) {
	t.Helper()

	if err := sketch.AddHash(hash); err != nil {
		t.Fatalf("AddHash(%016x): %v", hash, err)
	}
}

func splitmix64(x uint64) uint64 {
	return sketchtest.SplitMix64(x)
}
