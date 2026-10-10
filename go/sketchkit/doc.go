// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

// Package sketchkit supports counting distinct LLM users and finding token-heavy
// keys without per-user metric labels. Its subpackages build measurements that
// can be merged across workers and read in Go or Python.
//
// Canonicalize and keyed-hash identities with the canon and hash subpackages
// before adding them to a sketch. Use hllpp for distinct counts, frequentitems
// for token-weighted rankings, bloom for membership, minhash for set similarity,
// and summary for exchanging compatible window measurements.
package sketchkit
