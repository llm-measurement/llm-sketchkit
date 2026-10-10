// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

// Package hllpp estimates how many distinct users, sessions or prompts appeared
// in a window, using keyed hashes and fixed memory rather than a list of IDs.
// Merge compatible sketches to count across workers without summing overlapping
// distinct counts.
//
// The estimator follows "HyperLogLog in Practice: Algorithmic Engineering of
// a State of The Art Cardinality Estimation Algorithm" by Heule, Nunkesser,
// and Hall (EDBT 2013): https://research.google/pubs/pub40671/.
// The implementation uses the published alpha_m constants, empirical
// bias-correction tables, and HLL++ linear-counting thresholds for supported
// v0.1 precisions.
package hllpp
