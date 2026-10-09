// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

// Command malformed supplies deterministic decoder outcomes to the parity test.
package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/bloom"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/frequentitems"
	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/hllpp"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/minhash"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/summary"
)

type marshaler interface{ MarshalBinary() ([]byte, error) }
type outcome struct {
	Category string `json:"category"`
	Hex      string `json:"hex,omitempty"`
}

func classify(err error) string {
	if err == nil {
		return "accepted"
	}
	for _, target := range []error{hllpp.ErrUnknownProfile, frequentitems.ErrUnknownProfile, bloom.ErrUnknownProfile, minhash.ErrUnknownProfile} {
		if errors.Is(err, target) {
			return "profile"
		}
	}
	for _, target := range []error{hllpp.ErrInvalidPrecision, hllpp.ErrPrecisionMismatch, frequentitems.ErrInvalidMapSize, bloom.ErrInvalidShape, minhash.ErrInvalidSignatureLength} {
		if errors.Is(err, target) {
			return "shape"
		}
	}
	for _, target := range []error{sketchhash.ErrUnregisteredDomain, hllpp.ErrIncompatibleMerge, frequentitems.ErrIncompatibleMerge, bloom.ErrIncompatibleMerge, minhash.ErrIncompatibleMerge} {
		if errors.Is(err, target) {
			return "keying"
		}
	}
	return "wire"
}

func evaluate(kind string, data []byte) outcome {
	var s marshaler
	var err error
	switch kind {
	case "hllpp":
		s, err = hllpp.Parse(data)
	case "frequent_items":
		s, err = frequentitems.Parse(data)
	case "bloom":
		s, err = bloom.Parse(data)
	case "minhash":
		s, err = minhash.Parse(data)
	case "summary":
		s, err = summary.Parse(data)
	default:
		return outcome{Category: "unknown"}
	}
	if err != nil {
		if kind == "summary" {
			return outcome{Category: "summary"}
		}
		return outcome{Category: classify(err)}
	}
	b, err := s.MarshalBinary()
	if err != nil {
		return outcome{Category: "marshal"}
	}
	return outcome{Category: "accepted", Hex: hex.EncodeToString(b)}
}

func main() {
	scan := bufio.NewScanner(os.Stdin)
	scan.Buffer(make([]byte, 4096), 20<<20)
	out := json.NewEncoder(os.Stdout)
	for scan.Scan() {
		var row struct{ Kind, Hex string }
		if err := json.Unmarshal(scan.Bytes(), &row); err != nil {
			panic(err)
		}
		data, err := hex.DecodeString(row.Hex)
		if err != nil {
			panic(err)
		}
		if err := out.Encode(evaluate(row.Kind, data)); err != nil {
			panic(err)
		}
	}
	if err := scan.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
