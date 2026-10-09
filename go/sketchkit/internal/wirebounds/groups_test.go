// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

package wirebounds

import (
	"bytes"
	"google.golang.org/protobuf/encoding/protowire"
	"testing"
)

func TestGroups(t *testing.T) {
	for _, depth := range []int{1, 100, 101, 10002} {
		group := append(bytes.Repeat([]byte{0xa3, 6}, depth), bytes.Repeat([]byte{0xa4, 6}, depth)...)
		if Valid(group, Bloom) {
			t.Fatal("accepted top-level group")
		}
		for _, field := range []protowire.Number{1, 10, 11, 12, 13} {
			nested := protowire.AppendBytes(protowire.AppendTag(nil, field, 2), group)
			if Valid(nested, Bloom) {
				t.Fatal("accepted nested group")
			}
		}
		for _, field := range []protowire.Number{10, 11} {
			nested := protowire.AppendBytes(protowire.AppendTag(nil, 1, 2), group)
			nested = protowire.AppendBytes(protowire.AppendTag(nil, field, 2), nested)
			if Valid(nested, Bloom) {
				t.Fatal("accepted entry group")
			}
		}
	}
	// Byte fields remain opaque even when their bytes look like groups.
	bitset := protowire.AppendBytes(protowire.AppendTag(nil, 1, 2), []byte{3, 4})
	if !Valid(protowire.AppendBytes(protowire.AppendTag(nil, 12, 2), bitset), Bloom) {
		t.Fatal("interpreted opaque bytes")
	}
}
