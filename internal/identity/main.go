// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

// Command identity is a test oracle for canonical bytes, keyed hashes and estimates.
package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"unicode"
	"unicode/utf8"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/canon"
	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/hllpp"
	"golang.org/x/text/unicode/norm"
)

type estimateCase struct {
	Profile    string `json:"profile"`
	Count      int    `json:"count"`
	Seed       uint64 `json:"seed"`
	ForceDense bool   `json:"force_dense"`
}

type textResult struct {
	Canonical string `json:"canonical_hex"`
	Digest    string `json:"digest_hex"`
}

type estimateResult struct {
	Bits string `json:"estimate_bits"`
	Wire string `json:"wire_hex"`
}

func main() {
	properties := flag.Bool("properties", false, "emit Unicode 15 normalization properties for differential tests")
	flag.Parse()
	if *properties {
		if norm.Version != "15.0.0" || unicode.Version != "15.0.0" {
			fail(fmt.Errorf("Unicode 15 oracle required: norm=%s unicode=%s", norm.Version, unicode.Version))
		}
		rows := make([][]int, 0)
		for r := rune(0); r <= unicode.MaxRune; r++ {
			if !utf8.ValidRune(r) || !unicode.In(r, unicode.L, unicode.M, unicode.N, unicode.P, unicode.S, unicode.Z, unicode.Cc, unicode.Cf, unicode.Co) {
				continue
			}
			p := norm.NFC.PropertiesString(string(r))
			backward := 0
			if p.CCC() == 0 && !p.BoundaryBefore() {
				backward = 1
			}
			// Every row is an assigned Unicode 15 scalar, including inert ones.
			decomposes := 0
			if norm.NFKD.String(string(r)) != string(r) {
				decomposes = 1
			}
			rows = append(rows, []int{int(r), int(p.CCC()), backward, decomposes})
		}
		if err := json.NewEncoder(os.Stdout).Encode(rows); err != nil {
			fail(err)
		}
		return
	}
	var request struct {
		Texts     []string       `json:"texts"`
		Estimates []estimateCase `json:"estimates"`
	}
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		fail(err)
	}
	secret, err := sketchhash.SecretFromEnv("LLM_SKETCHKIT_IDENTITY_SECRET")
	if err != nil {
		fail(err)
	}
	result := struct {
		Texts     []textResult     `json:"texts"`
		Estimates []estimateResult `json:"estimates"`
	}{Texts: make([]textResult, 0, len(request.Texts)), Estimates: make([]estimateResult, 0, len(request.Estimates))}
	for _, s := range request.Texts {
		canonical, err := canon.CanonicalizeString(canon.TextV1, s)
		if err != nil {
			fail(err)
		}
		digest, err := sketchhash.Digest64Hex(secret, sketchhash.PromptV1, canonical)
		if err != nil {
			fail(err)
		}
		result.Texts = append(result.Texts, textResult{hex.EncodeToString(canonical), digest})
	}
	for _, c := range request.Estimates {
		s, err := hllpp.New(hllpp.Profile(c.Profile), sketchhash.PromptV1, sketchhash.HMACSHA25664)
		if err != nil {
			fail(err)
		}
		state := c.Seed
		for i := 0; i < c.Count; i++ {
			state += 0x9e3779b97f4a7c15
			z := state
			z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
			z = (z ^ (z >> 27)) * 0x94d049bb133111eb
			s.AddHash(z ^ (z >> 31))
		}
		if c.ForceDense {
			s.ForceDense()
		}
		wire, err := s.MarshalBinary()
		if err != nil {
			fail(err)
		}
		result.Estimates = append(result.Estimates, estimateResult{fmt.Sprintf("%016x", math.Float64bits(s.Estimate())), hex.EncodeToString(wire)})
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
