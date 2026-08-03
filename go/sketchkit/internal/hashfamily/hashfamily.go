// Code authors: Vijay Erramilli and Codex
// Package hashfamily implements the deterministic sketch-local hash family
// defined in spec/hash.md. It is not a keyed hash and must only be applied to
// already-keyed uint64 sketch inputs.
package hashfamily

const (
	BloomSeed   uint64 = 0x83984a98fd448a39
	MinHashSeed uint64 = 0xea59f2718f8069a6
)

// Mix64 is the spec-defined 64-bit SplitMix finalizer.
func Mix64(x uint64) uint64 {
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

// BloomHash returns the i-th Bloom hash-family value for hash.
func BloomHash(hash uint64, index uint32) uint64 {
	return Mix64(hash ^ Mix64(BloomSeed^uint64(index)))
}

// MinHash returns the i-th MinHash hash-family value for hash.
func MinHash(hash uint64, index uint32) uint64 {
	return Mix64(hash ^ Mix64(MinHashSeed^uint64(index)))
}
