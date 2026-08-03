// Code authors: Vijay Erramilli and Codex
// Package minhash implements the Go MinHash sketch for llm-sketchkit.
//
// The implementation uses a fixed-length signature and the deterministic
// sketch-local hash family defined in spec/hash.md. Signatures merge by
// element-wise minimum.
package minhash
