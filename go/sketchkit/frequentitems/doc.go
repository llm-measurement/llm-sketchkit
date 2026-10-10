// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

// Package frequentitems ranks the keys responsible for the most reported tokens
// or requests, with lower and upper bounds and fixed memory per sketch.
// Add keyed hashes of users, sessions or prompts with their recorded weight.
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
