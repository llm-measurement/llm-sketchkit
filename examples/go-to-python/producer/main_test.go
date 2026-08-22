// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay Erramilli and Codex
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/frequentitems"
	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/hllpp"
)

func TestGenerateProducesMergeableSummariesWithoutRawIDs(t *testing.T) {
	t.Setenv(secretEnvName, "test-only-not-a-deployment-secret-0123456789")
	secret, err := sketchhash.SecretFromEnv(secretEnvName)
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := generate(out, secret); err != nil {
		t.Fatal(err)
	}

	manifestData, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var gotManifest manifest
	if err := json.Unmarshal(manifestData, &gotManifest); err != nil {
		t.Fatal(err)
	}
	if len(gotManifest.Shards) != 4 {
		t.Fatalf("shards = %d, want 4", len(gotManifest.Shards))
	}

	validationData, err := os.ReadFile(filepath.Join(out, gotManifest.ValidationFile))
	if err != nil {
		t.Fatal(err)
	}
	var expected validation
	if err := json.Unmarshal(validationData, &expected); err != nil {
		t.Fatal(err)
	}

	for _, window := range expected.Windows {
		users, tokens := mergeWindow(t, out, gotManifest.Shards, window.Window)
		relativeError := math.Abs(users.Estimate()-float64(window.ExactDistinctUsers)) / float64(window.ExactDistinctUsers)
		if relativeError > 0.024375 {
			t.Fatalf("%s HLL++ relative error = %.6f, want <= 0.024375", window.Window, relativeError)
		}
		for _, item := range window.ExactTopTokenKeys {
			key := parseHexKey(t, item.KeyHex)
			if lower, upper := tokens.LowerBoundHash(key), tokens.UpperBoundHash(key); item.Tokens < lower || item.Tokens > upper {
				t.Fatalf("%s key %s exact %d outside [%d, %d]", window.Window, item.KeyHex, item.Tokens, lower, upper)
			}
		}
	}

	err = filepath.WalkDir(out, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if bytes.Contains(data, []byte("synthetic-user-")) {
			t.Fatalf("raw synthetic identifier found in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func mergeWindow(t *testing.T, out string, records []shardRecord, window string) (*hllpp.Sketch, *frequentitems.Sketch) {
	t.Helper()
	var users *hllpp.Sketch
	var tokens *frequentitems.Sketch
	for _, record := range records {
		if record.Window != window {
			continue
		}
		usersData, err := os.ReadFile(filepath.Join(out, record.UsersFile))
		if err != nil {
			t.Fatal(err)
		}
		userPart, err := hllpp.Parse(usersData)
		if err != nil {
			t.Fatal(err)
		}
		tokenData, err := os.ReadFile(filepath.Join(out, record.TokenItemsFile))
		if err != nil {
			t.Fatal(err)
		}
		tokenPart, err := frequentitems.Parse(tokenData)
		if err != nil {
			t.Fatal(err)
		}
		if users == nil {
			users, tokens = userPart, tokenPart
			continue
		}
		if err := users.Merge(userPart); err != nil {
			t.Fatal(err)
		}
		if err := tokens.Merge(tokenPart); err != nil {
			t.Fatal(err)
		}
	}
	if users == nil || tokens == nil {
		t.Fatalf("no records for %s", window)
	}
	return users, tokens
}

func parseHexKey(t *testing.T, value string) uint64 {
	t.Helper()
	var key uint64
	if _, err := fmt.Sscanf(value, "%x", &key); err != nil {
		t.Fatal(err)
	}
	return key
}
