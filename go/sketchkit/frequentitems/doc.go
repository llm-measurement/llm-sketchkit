// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay Erramilli and Codex
// Package frequentitems implements the Go weighted frequent-items sketch for
// llm-sketchkit.
//
// The implementation uses weighted Misra-Gries with a global error offset,
// deterministic pruning, and a preallocated counter pool. Merge
// semantics follow the mergeable-summary framing from Agarwal, Cormode, Huang,
// Phillips, Wei, and Yi, "Mergeable Summaries" (PODS 2012): summaries with
// identical metadata combine tracked residual mass, add carried error, then
// deterministically prune back to the configured bound.
//
// Serialization is deterministic for a fixed sketch state. Merge order is a
// semantic guarantee, not a byte-level state guarantee: merge-time pruning can
// leave different tracked sets and bytes for different valid merge orders, but
// bounds and frequent-item query-mode guarantees must still hold.
package frequentitems
