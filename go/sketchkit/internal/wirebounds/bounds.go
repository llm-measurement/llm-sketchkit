// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

// Package wirebounds checks allocation limits before protobuf decoding.
package wirebounds

import "google.golang.org/protobuf/encoding/protowire"

const (
	Global  = 4 << 20
	Small   = 64 << 10
	MinHash = 4 << 10
	Bloom   = 5 << 19
)

// Valid also bounds discarded oneof bodies. Otherwise a Bloom parser could
// allocate a large repeated HLL++ or frequent-items body before rejecting it.
func Valid(data []byte, limit int) bool {
	if len(data) > Global || len(data) > limit {
		return false
	}
	var sizes [4]int
	messages := 0
	entries := 0
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return false
		}
		data = data[n:]
		if typ == protowire.BytesType && (num == 1 || num >= 10 && num <= 13) {
			messages++
			if messages > 4096 {
				return false
			}
		}
		if typ == protowire.BytesType && num >= 10 && num <= 13 {
			body, used := protowire.ConsumeBytes(data)
			if used < 0 {
				return false
			}
			i := int(num - 10)
			sizes[i] += len(body) + 1
			caps := [4]int{Small, Small, Bloom, MinHash}
			if sizes[i] > caps[i] {
				return false
			}
			if num == 10 || num == 11 {
				count, ok := entryCount(body)
				entries += count
				if !ok || entries > 4096 {
					return false
				}
			}
		}
		n = protowire.ConsumeFieldValue(num, typ, data)
		if n < 0 {
			return false
		}
		data = data[n:]
	}
	return true
}

// Both repeated-message bodies share one budget, even if a later oneof field
// discards them. Supported canonical states need at most 2,048 such entries.
func entryCount(data []byte) (int, bool) {
	count := 0
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return count, false
		}
		data = data[n:]
		if num == 1 && typ == protowire.BytesType {
			count++
			if count > 4096 {
				return count, false
			}
		}
		n = protowire.ConsumeFieldValue(num, typ, data)
		if n < 0 {
			return count, false
		}
		data = data[n:]
	}
	return count, true
}
