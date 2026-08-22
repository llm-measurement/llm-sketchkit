// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay Erramilli and Codex
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/canon"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/frequentitems"
	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/hllpp"
)

const secretEnvName = "LLM_SKETCHKIT_SECRET"

type sketchPair struct {
	users  *hllpp.Sketch
	tokens *frequentitems.Sketch
}

type shardRecord struct {
	Window            string `json:"window"`
	Service           string `json:"service"`
	UsersFile         string `json:"users_file"`
	TokenItemsFile    string `json:"token_items_file"`
	ObservedRequests  int    `json:"observed_requests"`
	ObservedTokenMass int64  `json:"observed_token_mass"`
}

type manifest struct {
	FormatVersion        int           `json:"format_version"`
	HLLProfile           string        `json:"hll_profile"`
	FrequentItemsProfile string        `json:"frequent_items_profile"`
	HashDomain           string        `json:"hash_domain"`
	Shards               []shardRecord `json:"shards"`
	IncompatibleUsers    string        `json:"incompatible_users_file"`
	ValidationFile       string        `json:"synthetic_validation_file"`
}

type exactTokenKey struct {
	KeyHex string `json:"key_hex"`
	Tokens int64  `json:"tokens"`
}

type windowValidation struct {
	Window             string          `json:"window"`
	ExactDistinctUsers int             `json:"exact_distinct_users"`
	ExactTopTokenKeys  []exactTokenKey `json:"exact_top_token_keys"`
}

type validation struct {
	SyntheticOnly bool               `json:"synthetic_only"`
	Windows       []windowValidation `json:"windows"`
}

type exactWindow struct {
	users  map[uint64]struct{}
	tokens map[uint64]int64
}

func main() {
	out := flag.String("out", "examples/go-to-python/generated", "output directory")
	flag.Parse()

	secret, err := sketchhash.SecretFromEnv(secretEnvName)
	if err != nil {
		log.Fatal(err)
	}
	if err := generate(*out, secret); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote Go summaries to %s\n", *out)
}

func generate(out string, secret sketchhash.Secret) error {
	if err := os.MkdirAll(out, 0o700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	windows := []string{"window-1", "window-2"}
	services := []string{"api-east", "api-west"}
	pairs := make(map[string]*sketchPair, len(windows)*len(services))
	records := make(map[string]*shardRecord, len(windows)*len(services))
	exact := make(map[string]*exactWindow, len(windows))

	for _, window := range windows {
		exact[window] = &exactWindow{users: map[uint64]struct{}{}, tokens: map[uint64]int64{}}
		for _, service := range services {
			key := window + "/" + service
			pair, err := newSketchPair()
			if err != nil {
				return err
			}
			pairs[key] = pair
			records[key] = &shardRecord{Window: window, Service: service}
		}
	}

	for windowIndex, window := range windows {
		for rank := range 900 {
			userNumber := rank + windowIndex*200
			repeats := 1
			if rank < 24 {
				repeats += (24 - rank) * 7
			}
			if windowIndex == 1 && rank >= 4 && rank < 8 {
				repeats += 80
			}

			canonical, err := canon.CanonicalizeString(canon.TextV1, fmt.Sprintf("synthetic-user-%04d", userNumber))
			if err != nil {
				return fmt.Errorf("canonicalize synthetic user: %w", err)
			}
			userKey, err := sketchhash.Hash64(secret, sketchhash.UserV1, canonical)
			if err != nil {
				return fmt.Errorf("hash synthetic user: %w", err)
			}

			for request := range repeats {
				service := services[(rank+request+windowIndex)%len(services)]
				tokens := int64(80 + (rank*37+request*17+windowIndex*53)%480)
				if windowIndex == 1 && rank == 5 {
					tokens += 900
				}
				key := window + "/" + service
				pairs[key].users.AddHash(userKey)
				if err := pairs[key].tokens.AddHash(userKey, tokens); err != nil {
					return fmt.Errorf("add token weight: %w", err)
				}
				records[key].ObservedRequests++
				records[key].ObservedTokenMass += tokens
				exact[window].users[userKey] = struct{}{}
				exact[window].tokens[userKey] += tokens
			}
		}
	}

	outputManifest := manifest{
		FormatVersion:        1,
		HLLProfile:           string(hllpp.ProfileSmall),
		FrequentItemsProfile: string(frequentitems.ProfileMicro),
		HashDomain:           string(sketchhash.UserV1),
		IncompatibleUsers:    "incompatible-users-micro.hll.pb",
		ValidationFile:       "synthetic-validation.json",
	}
	for _, window := range windows {
		for _, service := range services {
			key := window + "/" + service
			record := records[key]
			record.UsersFile = fmt.Sprintf("%s-%s-users.hll.pb", window, service)
			record.TokenItemsFile = fmt.Sprintf("%s-%s-token-items.pb", window, service)
			if err := writeSketches(out, *record, pairs[key]); err != nil {
				return err
			}
			outputManifest.Shards = append(outputManifest.Shards, *record)
		}
	}

	incompatible, err := hllpp.New(hllpp.ProfileMicro, sketchhash.UserV1, sketchhash.HMACSHA25664)
	if err != nil {
		return fmt.Errorf("create incompatible sketch: %w", err)
	}
	if err := writeBinary(filepath.Join(out, outputManifest.IncompatibleUsers), incompatible); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(out, "manifest.json"), outputManifest); err != nil {
		return err
	}

	result := validation{SyntheticOnly: true}
	for _, window := range windows {
		result.Windows = append(result.Windows, windowValidation{
			Window:             window,
			ExactDistinctUsers: len(exact[window].users),
			ExactTopTokenKeys:  topTokenKeys(exact[window].tokens, 20),
		})
	}
	return writeJSON(filepath.Join(out, outputManifest.ValidationFile), result)
}

func newSketchPair() (*sketchPair, error) {
	users, err := hllpp.New(hllpp.ProfileSmall, sketchhash.UserV1, sketchhash.HMACSHA25664)
	if err != nil {
		return nil, fmt.Errorf("create users sketch: %w", err)
	}
	tokens, err := frequentitems.New(frequentitems.ProfileMicro, sketchhash.UserV1, sketchhash.HMACSHA25664)
	if err != nil {
		return nil, fmt.Errorf("create token sketch: %w", err)
	}
	return &sketchPair{users: users, tokens: tokens}, nil
}

func writeSketches(out string, record shardRecord, pair *sketchPair) error {
	if err := writeBinary(filepath.Join(out, record.UsersFile), pair.users); err != nil {
		return err
	}
	return writeBinary(filepath.Join(out, record.TokenItemsFile), pair.tokens)
}

type binaryMarshaler interface {
	MarshalBinary() ([]byte, error)
}

func writeBinary(path string, value binaryMarshaler) error {
	data, err := value.MarshalBinary()
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func topTokenKeys(tokens map[uint64]int64, limit int) []exactTokenKey {
	items := make([]exactTokenKey, 0, len(tokens))
	for key, weight := range tokens {
		items = append(items, exactTokenKey{KeyHex: fmt.Sprintf("%016x", key), Tokens: weight})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Tokens != items[j].Tokens {
			return items[i].Tokens > items[j].Tokens
		}
		return items[i].KeyHex < items[j].KeyHex
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}
