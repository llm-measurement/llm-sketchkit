// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay Erramilli and Codex
package fuzz_test

import (
	"bytes"
	"encoding/hex"
	"testing"
	"unicode/utf8"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/bloom"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/canon"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/frequentitems"
	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/hllpp"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/minhash"
)

const maxFuzzUpdateBytes = 256

type wireValue interface {
	MarshalBinary() ([]byte, error)
}

func FuzzCanonicalization(f *testing.F) {
	f.Add([]byte("  cafe\xcc\x81\r\n"))
	f.Add([]byte("\xffinvalid"))
	f.Add([]byte("\talpha\rbeta  "))

	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 1<<20 {
			t.Skip()
		}
		canonical, err := canon.Canonicalize(canon.TextV1, input)
		if err != nil {
			if utf8.Valid(input) {
				t.Fatalf("valid UTF-8 was rejected: %v", err)
			}
			return
		}
		if !utf8.Valid(canonical) {
			t.Fatal("canonical output is not valid UTF-8")
		}
		again, err := canon.Canonicalize(canon.TextV1, canonical)
		if err != nil {
			t.Fatalf("canonical output was rejected: %v", err)
		}
		if !bytes.Equal(canonical, again) {
			t.Fatal("canonicalization is not idempotent")
		}
	})
}

func FuzzUntrustedParse(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0xff, 0xff, 0xff})
	f.Add(mustDecodeHex("0a20080110011a056d6963726f220970726f6d70743a763128013001a0010ca8011052120a04081810310a04082210310a0408241031"))
	f.Add(mustDecodeHex("0a1d080210011a056d6963726f220970726f6d70743a763128013003b00180025a160a0c091000000000000000100c0a040920000000000000001003180f"))

	parsers := []struct {
		name  string
		parse func([]byte) (wireValue, error)
	}{
		{name: "hllpp", parse: func(data []byte) (wireValue, error) { return hllpp.Parse(data) }},
		{name: "frequent-items", parse: func(data []byte) (wireValue, error) { return frequentitems.Parse(data) }},
		{name: "bloom", parse: func(data []byte) (wireValue, error) { return bloom.Parse(data) }},
		{name: "minhash", parse: func(data []byte) (wireValue, error) { return minhash.Parse(data) }},
	}

	f.Fuzz(func(t *testing.T, input []byte) {
		for _, parser := range parsers {
			value, err := parser.parse(input)
			if err != nil {
				continue
			}
			first, err := value.MarshalBinary()
			if err != nil {
				t.Fatalf("%s marshal after parse: %v", parser.name, err)
			}
			parsed, err := parser.parse(first)
			if err != nil {
				t.Fatalf("%s rejected its own encoding: %v", parser.name, err)
			}
			second, err := parsed.MarshalBinary()
			if err != nil {
				t.Fatalf("%s second marshal: %v", parser.name, err)
			}
			if !bytes.Equal(first, second) {
				t.Fatalf("%s parse and marshal is not stable", parser.name)
			}
		}
	})
}

func FuzzCompatibleMerges(f *testing.F) {
	f.Add([]byte("left"), []byte("right"))
	f.Add(bytes.Repeat([]byte{0x01}, maxFuzzUpdateBytes), bytes.Repeat([]byte{0xff}, maxFuzzUpdateBytes))

	f.Fuzz(func(t *testing.T, leftInput []byte, rightInput []byte) {
		left := buildSketches(t, leftInput)
		right := buildSketches(t, rightInput)

		checkHLLPPMerge(t, left.hllpp, right.hllpp)
		checkFrequentItemsMerge(t, left.frequentItems, right.frequentItems)
		checkBloomMerge(t, left.bloom, right.bloom)
		checkMinHashMerge(t, left.minhash, right.minhash)
	})
}

type sketches struct {
	hllpp         *hllpp.Sketch
	frequentItems *frequentitems.Sketch
	bloom         *bloom.Sketch
	minhash       *minhash.Sketch
}

func buildSketches(t *testing.T, input []byte) sketches {
	t.Helper()
	hllppSketch, err := hllpp.New(hllpp.ProfileMicro, sketchhash.PromptV1, sketchhash.HMACSHA25664)
	if err != nil {
		t.Fatal(err)
	}
	frequentItemsSketch, err := frequentitems.New(
		frequentitems.ProfileMicro,
		sketchhash.PromptV1,
		sketchhash.HMACSHA25664,
	)
	if err != nil {
		t.Fatal(err)
	}
	bloomSketch, err := bloom.New(bloom.ProfileMicro, sketchhash.PromptV1, sketchhash.HMACSHA25664)
	if err != nil {
		t.Fatal(err)
	}
	minHashSketch, err := minhash.New(minhash.ProfileMicro, sketchhash.PromptV1, sketchhash.HMACSHA25664)
	if err != nil {
		t.Fatal(err)
	}

	if len(input) > maxFuzzUpdateBytes {
		input = input[:maxFuzzUpdateBytes]
	}
	for index, current := range input {
		value := uint64(index)<<8 | uint64(current)
		hllppSketch.AddHash(value)
		if err := frequentItemsSketch.AddHash(value, int64(current%31)+1); err != nil {
			t.Fatal(err)
		}
		if err := bloomSketch.AddHash(value); err != nil {
			t.Fatal(err)
		}
		if err := minHashSketch.AddHash(value); err != nil {
			t.Fatal(err)
		}
	}

	return sketches{
		hllpp:         hllppSketch,
		frequentItems: frequentItemsSketch,
		bloom:         bloomSketch,
		minhash:       minHashSketch,
	}
}

func checkHLLPPMerge(t *testing.T, left *hllpp.Sketch, right *hllpp.Sketch) {
	t.Helper()
	rightBefore := marshal(t, right)
	leftThenRight := left.Clone()
	if err := leftThenRight.Merge(right); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rightBefore, marshal(t, right)) {
		t.Fatal("HLL++ merge mutated its source")
	}
	rightThenLeft := right.Clone()
	if err := rightThenLeft.Merge(left); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(marshal(t, leftThenRight), marshal(t, rightThenLeft)) {
		t.Fatal("HLL++ merge is not commutative")
	}
	assertHLLPPRoundTrip(t, leftThenRight)
}

func checkFrequentItemsMerge(t *testing.T, left *frequentitems.Sketch, right *frequentitems.Sketch) {
	t.Helper()
	rightBefore := marshal(t, right)
	merged := left.Clone()
	if err := merged.Merge(right); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rightBefore, marshal(t, right)) {
		t.Fatal("frequent-items merge mutated its source")
	}
	if merged.TotalWeight() != left.TotalWeight()+right.TotalWeight() {
		t.Fatal("frequent-items merge lost total weight")
	}
	if merged.Len() > merged.MapSize() {
		t.Fatal("frequent-items merge exceeded its map size")
	}
	items, err := merged.FrequentItems(frequentitems.NoFalseNegatives)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.LowerBound > item.Estimate || item.Estimate > item.UpperBound {
			t.Fatal("frequent-items bounds do not bracket the estimate")
		}
	}
	assertFrequentItemsRoundTrip(t, merged)
}

func checkBloomMerge(t *testing.T, left *bloom.Sketch, right *bloom.Sketch) {
	t.Helper()
	rightBefore := marshal(t, right)
	leftThenRight := left.Clone()
	if err := leftThenRight.Merge(right); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rightBefore, marshal(t, right)) {
		t.Fatal("Bloom merge mutated its source")
	}
	rightThenLeft := right.Clone()
	if err := rightThenLeft.Merge(left); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(marshal(t, leftThenRight), marshal(t, rightThenLeft)) {
		t.Fatal("Bloom merge is not commutative")
	}
	assertBloomRoundTrip(t, leftThenRight)
}

func checkMinHashMerge(t *testing.T, left *minhash.Sketch, right *minhash.Sketch) {
	t.Helper()
	rightBefore := marshal(t, right)
	leftThenRight := left.Clone()
	if err := leftThenRight.Merge(right); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rightBefore, marshal(t, right)) {
		t.Fatal("MinHash merge mutated its source")
	}
	rightThenLeft := right.Clone()
	if err := rightThenLeft.Merge(left); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(marshal(t, leftThenRight), marshal(t, rightThenLeft)) {
		t.Fatal("MinHash merge is not commutative")
	}
	assertMinHashRoundTrip(t, leftThenRight)
}

func assertHLLPPRoundTrip(t *testing.T, value *hllpp.Sketch) {
	t.Helper()
	encoded := marshal(t, value)
	parsed, err := hllpp.Parse(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, marshal(t, parsed)) {
		t.Fatal("HLL++ round trip changed bytes")
	}
}

func assertFrequentItemsRoundTrip(t *testing.T, value *frequentitems.Sketch) {
	t.Helper()
	encoded := marshal(t, value)
	parsed, err := frequentitems.Parse(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, marshal(t, parsed)) {
		t.Fatal("frequent-items round trip changed bytes")
	}
}

func assertBloomRoundTrip(t *testing.T, value *bloom.Sketch) {
	t.Helper()
	encoded := marshal(t, value)
	parsed, err := bloom.Parse(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, marshal(t, parsed)) {
		t.Fatal("Bloom round trip changed bytes")
	}
}

func assertMinHashRoundTrip(t *testing.T, value *minhash.Sketch) {
	t.Helper()
	encoded := marshal(t, value)
	parsed, err := minhash.Parse(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, marshal(t, parsed)) {
		t.Fatal("MinHash round trip changed bytes")
	}
}

func marshal(t *testing.T, value wireValue) []byte {
	t.Helper()
	encoded, err := value.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func mustDecodeHex(value string) []byte {
	decoded, err := hex.DecodeString(value)
	if err != nil {
		panic(err)
	}
	return decoded
}
