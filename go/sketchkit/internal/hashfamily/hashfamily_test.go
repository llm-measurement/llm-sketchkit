// Code authors: Vijay Erramilli and Codex
package hashfamily

import (
	"crypto/sha256"
	"encoding/binary"
	"testing"
)

func TestSeedConstantsDerivedFromSpecTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tag  string
		want uint64
	}{
		{
			name: "bloom",
			tag:  "llm-sketchkit:bloom:v1",
			want: BloomSeed,
		},
		{
			name: "minhash",
			tag:  "llm-sketchkit:minhash:v1",
			want: MinHashSeed,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			digest := sha256.Sum256([]byte(test.tag))
			got := binary.BigEndian.Uint64(digest[:8])
			if got != test.want {
				t.Fatalf("seed(%q) = %#016x, want %#016x", test.tag, got, test.want)
			}
		})
	}
}
