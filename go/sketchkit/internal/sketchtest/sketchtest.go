// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

// Package sketchtest holds shared fixture and serialization checks for tests.
package sketchtest

import (
	"encoding"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func ReadJSON[T any](t testing.TB, path string) T {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read vector %s: %v", path, err)
	}
	var vector T
	if err := json.Unmarshal(data, &vector); err != nil {
		t.Fatalf("decode vector %s: %v", path, err)
	}
	return vector
}

func ParseHashHex(t testing.TB, value string) uint64 {
	t.Helper()
	data, err := hex.DecodeString(value)
	if err != nil {
		t.Fatalf("decode hash %q: %v", value, err)
	}
	if len(data) != 8 {
		t.Fatalf("hash %q decoded to %d bytes, want 8", value, len(data))
	}
	return binary.BigEndian.Uint64(data)
}

func SplitMix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

func AssertStable[S encoding.BinaryMarshaler](t testing.TB, sketch S, parse func([]byte) (S, error)) {
	t.Helper()
	first, err := sketch.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary(): %v", err)
	}
	parsed, err := parse(first)
	if err != nil {
		t.Fatalf("Parse(): %v", err)
	}
	second, err := parsed.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() after parse: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("reserialization changed bytes:\nfirst=%s\nsecond=%s", hex.EncodeToString(first), hex.EncodeToString(second))
	}
}

func AssertSerializedHex[S encoding.BinaryMarshaler](t testing.TB, sketch S, want string, parse func([]byte) (S, error)) {
	t.Helper()
	if want == "" {
		return
	}
	encoded, err := sketch.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary(): %v", err)
	}
	if got := hex.EncodeToString(encoded); got != want {
		t.Fatalf("serialized hex = %s, want %s", got, want)
	}
	decoded, err := hex.DecodeString(want)
	if err != nil {
		t.Fatalf("decode serialized hex: %v", err)
	}
	parsed, err := parse(decoded)
	if err != nil {
		t.Fatalf("Parse(serialized_hex): %v", err)
	}
	reencoded, err := parsed.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() after serialized_hex parse: %v", err)
	}
	if hex.EncodeToString(reencoded) != want {
		t.Fatal("serialized_hex parse/reencode changed bytes")
	}
}
