// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay Erramilli and Codex
// Package hllpp implements the Go HLL++ sketch for llm-sketchkit.
//
// The estimator follows "HyperLogLog in Practice: Algorithmic Engineering of
// a State of The Art Cardinality Estimation Algorithm" by Heule, Nunkesser,
// and Hall (EDBT 2013): https://research.google/pubs/pub40671/.
// The implementation uses the published alpha_m constants, empirical
// bias-correction tables, and HLL++ linear-counting thresholds for supported
// v0.1 precisions.
package hllpp
