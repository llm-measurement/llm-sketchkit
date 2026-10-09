// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

package sketchcheck_test

import (
	"errors"
	"strings"
	"testing"

	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
	sketchpb "github.com/llm-measurement/llm-sketchkit/go/sketchkit/internal/pb"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/internal/sketchcheck"
)

func TestHeaderErrorOrder(t *testing.T) {
	invalid := errors.New("invalid wire")
	metadata := &sketchpb.SketchMetadata{
		Kind: sketchpb.SketchKind(99), WireVersion: 2,
		HashAlgo: sketchpb.HashAlgorithm(99),
	}
	check := func(want string) {
		t.Helper()
		err := sketchcheck.Header(metadata, sketchpb.SketchKind_SKETCH_KIND_HLLPP, 1, invalid)
		if err == nil || !errors.Is(err, invalid) || err.Error() != want {
			t.Fatalf("Header() = %v, want wrapped %q", err, want)
		}
	}
	check("invalid wire: kind 99")
	metadata.Kind = sketchpb.SketchKind_SKETCH_KIND_HLLPP
	check("invalid wire: wire version 2")
	metadata.WireVersion = 1
	check("invalid wire: hash algorithm 99")
	metadata.HashAlgo = sketchpb.HashAlgorithm_HASH_ALGORITHM_HMAC_SHA256_64
	if err := sketchcheck.Header(metadata, metadata.Kind, 1, invalid); err != nil {
		t.Fatal(err)
	}
}

func TestKeyingErrorOrderAndPrivacy(t *testing.T) {
	incompatible := errors.New("incompatible merge")
	sentinel := strings.Repeat("PRIVATE_KEYING_SENTINEL\n\x1b", 32)
	err := sketchcheck.Keying(sketchhash.Domain(sentinel), sketchhash.Algorithm(sentinel), incompatible)
	if err != sketchhash.ErrUnregisteredDomain {
		t.Fatalf("Keying() = %v, want unregistered domain sentinel", err)
	}
	err = sketchcheck.Keying(sketchhash.PromptV1, sketchhash.Algorithm(sentinel), incompatible)
	if !errors.Is(err, incompatible) || err.Error() != "incompatible merge: unsupported hash algorithm" {
		t.Fatalf("Keying() = %v, want fixed wrapped algorithm error", err)
	}
	if err := sketchcheck.Keying(sketchhash.PromptV1, sketchhash.HMACSHA25664, incompatible); err != nil {
		t.Fatal(err)
	}
}
