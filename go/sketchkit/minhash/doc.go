// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

// Package minhash estimates how much two sets of keyed prompts, tools or
// documents overlap, using fixed-size signatures that can be merged.
//
// The implementation uses a fixed-length signature and the deterministic
// sketch-local hash family defined in spec/hash.md. Signatures merge by
// element-wise minimum.
package minhash
