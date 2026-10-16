// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

package sketchkit_test

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/bloom"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/frequentitems"
	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/hllpp"
	sketchpb "github.com/llm-measurement/llm-sketchkit/go/sketchkit/internal/pb"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/minhash"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/summary"
	"google.golang.org/protobuf/proto"
)

func TestMetadataErrorHygiene(t *testing.T) {
	sentinel := strings.Repeat("PRIVATE_WP3_SENTINEL\n\x1b[31m", 32)
	constructors := []struct {
		name    string
		new     func(string, sketchhash.Domain, sketchhash.Algorithm) (encoding.BinaryMarshaler, error)
		parse   func([]byte) error
		profile error
		merge   error
	}{
		{"hllpp", func(p string, d sketchhash.Domain, a sketchhash.Algorithm) (encoding.BinaryMarshaler, error) {
			return hllpp.New(hllpp.Profile(p), d, a)
		}, func(b []byte) error { _, err := hllpp.Parse(b); return err }, hllpp.ErrUnknownProfile, hllpp.ErrIncompatibleMerge},
		{"frequent_items", func(p string, d sketchhash.Domain, a sketchhash.Algorithm) (encoding.BinaryMarshaler, error) {
			return frequentitems.New(frequentitems.Profile(p), d, a)
		}, func(b []byte) error { _, err := frequentitems.Parse(b); return err }, frequentitems.ErrUnknownProfile, frequentitems.ErrIncompatibleMerge},
		{"bloom", func(p string, d sketchhash.Domain, a sketchhash.Algorithm) (encoding.BinaryMarshaler, error) {
			return bloom.New(bloom.Profile(p), d, a)
		}, func(b []byte) error { _, err := bloom.Parse(b); return err }, bloom.ErrUnknownProfile, bloom.ErrIncompatibleMerge},
		{"minhash", func(p string, d sketchhash.Domain, a sketchhash.Algorithm) (encoding.BinaryMarshaler, error) {
			return minhash.New(minhash.Profile(p), d, a)
		}, func(b []byte) error { _, err := minhash.Parse(b); return err }, minhash.ErrUnknownProfile, minhash.ErrIncompatibleMerge},
	}
	fixture, err := os.ReadFile("../../vectors/summaries/envelope.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range constructors {
		t.Run(kind.name, func(t *testing.T) {
			for _, field := range []string{"profile", "domain", "algorithm"} {
				t.Run(field, func(t *testing.T) {
					profile, domain, algorithm := "micro", sketchhash.PromptV1, sketchhash.HMACSHA25664
					want := kind.profile
					switch field {
					case "profile":
						profile = sentinel
					case "domain":
						domain, want = sketchhash.Domain(sentinel), sketchhash.ErrUnregisteredDomain
					case "algorithm":
						algorithm, want = sketchhash.Algorithm(sentinel), kind.merge
					}
					_, err := kind.new(profile, domain, algorithm)
					assertSafeMetadataError(t, err, want)
					if field == "algorithm" {
						return // Wire algorithms are enums, not attacker-controlled strings.
					}
					sketch, err := kind.new("micro", sketchhash.PromptV1, sketchhash.HMACSHA25664)
					if err != nil {
						t.Fatal(err)
					}
					data, err := sketch.MarshalBinary()
					if err != nil {
						t.Fatal(err)
					}
					var wire sketchpb.Sketch
					if err := proto.Unmarshal(data, &wire); err != nil {
						t.Fatal(err)
					}
					wire.Metadata.Profile, wire.Metadata.HashDomain = profile, string(domain)
					data, err = proto.Marshal(&wire)
					if err != nil {
						t.Fatal(err)
					}
					assertSafeMetadataError(t, kind.parse(data), want)

					envelope, err := summary.Parse(fixture)
					if err != nil {
						t.Fatal(err)
					}
					envelope.Sketches[kind.name] = summary.Payload{Kind: kind.name, Data: data}
					assertSafeMetadataError(t, envelope.Validate(), nil)
					_, err = envelope.MarshalBinary()
					assertSafeMetadataError(t, err, nil)
					encoded, err := json.Marshal(envelope)
					if err != nil {
						t.Fatal(err)
					}
					_, err = summary.Parse(encoded)
					assertSafeMetadataError(t, err, nil)
					_, err = summary.Combine([]summary.Envelope{envelope}, []string{envelope.ProducerID})
					assertSafeMetadataError(t, err, nil)
				})
			}
		})
	}
}

func assertSafeMetadataError(t *testing.T, err, want error) {
	t.Helper()
	if err == nil || (want != nil && !errors.Is(err, want)) {
		t.Fatalf("error category = %v, want %v", err, want)
	}
	for _, message := range []string{err.Error(), fmt.Sprintf("%+v", err)} {
		if len(message) > 160 || strings.Contains(message, "PRIVATE_WP3_SENTINEL") || strings.ContainsAny(message, "\n\r\x1b") {
			t.Fatal("error exposed untrusted metadata")
		}
	}
}
