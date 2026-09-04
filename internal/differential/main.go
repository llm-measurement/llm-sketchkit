// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay Erramilli and Codex
package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/bloom"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/canon"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/frequentitems"
	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/hllpp"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/minhash"
)

const (
	secretEnvironment = "LLM_SKETCHKIT_DIFFERENTIAL_SECRET"
	maxInputBytes     = 32 << 20
	maxCases          = 100
	maxPartitions     = 4
	maxUpdatesPerCase = 2_000
	maxTextBytes      = 4 << 10
)

type request struct {
	Cases []testCase `json:"cases"`
}

type testCase struct {
	Profile    string     `json:"profile"`
	Partitions [][]update `json:"partitions"`
}

type update struct {
	Text   string `json:"text"`
	Weight int64  `json:"weight"`
}

type response struct {
	Cases []result `json:"cases"`
}

type result struct {
	HLLPPHex         string `json:"hllpp_hex"`
	FrequentItemsHex string `json:"frequent_items_hex"`
	BloomHex         string `json:"bloom_hex"`
	MinHashHex       string `json:"minhash_hex"`
}

type partitionSketches struct {
	hllpp         *hllpp.Sketch
	frequentItems *frequentitems.Sketch
	bloom         *bloom.Sketch
	minhash       *minhash.Sketch
}

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(input io.Reader, output io.Writer) error {
	payload, err := io.ReadAll(io.LimitReader(input, maxInputBytes+1))
	if err != nil {
		return fmt.Errorf("read request: %w", err)
	}
	if len(payload) > maxInputBytes {
		return fmt.Errorf("request exceeds %d bytes", maxInputBytes)
	}

	var requestValue request
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&requestValue); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("decode request: trailing JSON value")
	}
	if len(requestValue.Cases) == 0 || len(requestValue.Cases) > maxCases {
		return fmt.Errorf("case count must be between 1 and %d", maxCases)
	}

	secret, err := sketchhash.SecretFromEnv(secretEnvironment)
	if err != nil {
		return fmt.Errorf("load differential secret: %w", err)
	}

	responseValue := response{Cases: make([]result, 0, len(requestValue.Cases))}
	for index, caseValue := range requestValue.Cases {
		caseResult, err := evaluateCase(secret, caseValue)
		if err != nil {
			return fmt.Errorf("case %d: %w", index, err)
		}
		responseValue.Cases = append(responseValue.Cases, caseResult)
	}

	encoder := json.NewEncoder(output)
	if err := encoder.Encode(responseValue); err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	return nil
}

func evaluateCase(secret sketchhash.Secret, caseValue testCase) (result, error) {
	if len(caseValue.Partitions) == 0 || len(caseValue.Partitions) > maxPartitions {
		return result{}, fmt.Errorf("partition count must be between 1 and %d", maxPartitions)
	}

	parts := make([]partitionSketches, 0, len(caseValue.Partitions))
	totalUpdates := 0
	for _, updates := range caseValue.Partitions {
		totalUpdates += len(updates)
		if totalUpdates > maxUpdatesPerCase {
			return result{}, fmt.Errorf("update count exceeds %d", maxUpdatesPerCase)
		}

		part, err := newPartitionSketches(caseValue.Profile)
		if err != nil {
			return result{}, err
		}
		for _, current := range updates {
			if len(current.Text) > maxTextBytes {
				return result{}, fmt.Errorf("text exceeds %d bytes", maxTextBytes)
			}
			if current.Weight < 0 {
				return result{}, errors.New("negative update weight")
			}
			canonical, err := canon.CanonicalizeString(canon.TextV1, current.Text)
			if err != nil {
				return result{}, fmt.Errorf("canonicalize text: %w", err)
			}
			digest, err := sketchhash.Hash64(secret, sketchhash.PromptV1, canonical)
			if err != nil {
				return result{}, fmt.Errorf("hash text: %w", err)
			}
			part.hllpp.AddHash(digest)
			if err := part.frequentItems.AddHash(digest, current.Weight); err != nil {
				return result{}, fmt.Errorf("update frequent-items: %w", err)
			}
			if err := part.bloom.AddHash(digest); err != nil {
				return result{}, fmt.Errorf("update Bloom: %w", err)
			}
			if err := part.minhash.AddHash(digest); err != nil {
				return result{}, fmt.Errorf("update MinHash: %w", err)
			}
		}
		parts = append(parts, part)
	}

	merged := clonePartition(parts[0])
	for _, part := range parts[1:] {
		if err := mergePartition(&merged, part); err != nil {
			return result{}, err
		}
	}

	hllppBytes, err := merged.hllpp.MarshalBinary()
	if err != nil {
		return result{}, fmt.Errorf("serialize HLL++: %w", err)
	}
	frequentItemsBytes, err := merged.frequentItems.MarshalBinary()
	if err != nil {
		return result{}, fmt.Errorf("serialize frequent-items: %w", err)
	}
	bloomBytes, err := merged.bloom.MarshalBinary()
	if err != nil {
		return result{}, fmt.Errorf("serialize Bloom: %w", err)
	}
	minHashBytes, err := merged.minhash.MarshalBinary()
	if err != nil {
		return result{}, fmt.Errorf("serialize MinHash: %w", err)
	}

	return result{
		HLLPPHex:         hex.EncodeToString(hllppBytes),
		FrequentItemsHex: hex.EncodeToString(frequentItemsBytes),
		BloomHex:         hex.EncodeToString(bloomBytes),
		MinHashHex:       hex.EncodeToString(minHashBytes),
	}, nil
}

func newPartitionSketches(profile string) (partitionSketches, error) {
	if profile != "micro" && profile != "small" {
		return partitionSketches{}, fmt.Errorf("unsupported differential profile %q", profile)
	}
	hllppSketch, err := hllpp.New(hllpp.Profile(profile), sketchhash.PromptV1, sketchhash.HMACSHA25664)
	if err != nil {
		return partitionSketches{}, fmt.Errorf("construct HLL++: %w", err)
	}
	frequentItemsSketch, err := frequentitems.New(
		frequentitems.Profile(profile),
		sketchhash.PromptV1,
		sketchhash.HMACSHA25664,
	)
	if err != nil {
		return partitionSketches{}, fmt.Errorf("construct frequent-items: %w", err)
	}
	bloomSketch, err := bloom.New(bloom.Profile(profile), sketchhash.PromptV1, sketchhash.HMACSHA25664)
	if err != nil {
		return partitionSketches{}, fmt.Errorf("construct Bloom: %w", err)
	}
	minHashSketch, err := minhash.New(minhash.Profile(profile), sketchhash.PromptV1, sketchhash.HMACSHA25664)
	if err != nil {
		return partitionSketches{}, fmt.Errorf("construct MinHash: %w", err)
	}

	return partitionSketches{
		hllpp:         hllppSketch,
		frequentItems: frequentItemsSketch,
		bloom:         bloomSketch,
		minhash:       minHashSketch,
	}, nil
}

func clonePartition(source partitionSketches) partitionSketches {
	return partitionSketches{
		hllpp:         source.hllpp.Clone(),
		frequentItems: source.frequentItems.Clone(),
		bloom:         source.bloom.Clone(),
		minhash:       source.minhash.Clone(),
	}
}

func mergePartition(destination *partitionSketches, source partitionSketches) error {
	if err := destination.hllpp.Merge(source.hllpp); err != nil {
		return fmt.Errorf("merge HLL++: %w", err)
	}
	if err := destination.frequentItems.Merge(source.frequentItems); err != nil {
		return fmt.Errorf("merge frequent-items: %w", err)
	}
	if err := destination.bloom.Merge(source.bloom); err != nil {
		return fmt.Errorf("merge Bloom: %w", err)
	}
	if err := destination.minhash.Merge(source.minhash); err != nil {
		return fmt.Errorf("merge MinHash: %w", err)
	}
	return nil
}
