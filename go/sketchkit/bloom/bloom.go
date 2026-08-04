// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay Erramilli and Codex
package bloom

import (
	"errors"
	"fmt"
	"math"
	"math/bits"

	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/internal/hashfamily"
	sketchpb "github.com/llm-measurement/llm-sketchkit/go/sketchkit/internal/pb"
	"google.golang.org/protobuf/proto"
)

const (
	wireVersion  = 1
	maxWireBytes = 4 << 20
)

// Profile names a Bloom profile from spec/profiles.md.
type Profile string

const (
	ProfileMicro   Profile = "micro"
	ProfileSmall   Profile = "small"
	ProfileDefault Profile = "default"
)

var (
	// ErrUnknownProfile reports a profile outside spec/profiles.md.
	ErrUnknownProfile = errors.New("unknown bloom profile")

	// ErrInvalidShape reports a non-profile Bloom bit/hash-count pair.
	ErrInvalidShape = errors.New("invalid bloom shape")

	// ErrIncompatibleMerge reports metadata mismatch on merge.
	ErrIncompatibleMerge = errors.New("incompatible bloom merge")

	// ErrCountOverflow reports inserted-count overflow.
	ErrCountOverflow = errors.New("bloom inserted count overflow")

	// ErrInvalidWireEncoding reports a malformed serialized sketch.
	ErrInvalidWireEncoding = errors.New("invalid bloom wire encoding")
)

type profileConfig struct {
	ratedInsertions uint64
	targetFPR       float64
	bitCount        uint64
	hashCount       uint32
}

var profileConfigs = map[Profile]profileConfig{
	ProfileMicro:   {ratedInsertions: 10_000, targetFPR: 0.001, bitCount: 143_776, hashCount: 10},
	ProfileSmall:   {ratedInsertions: 100_000, targetFPR: 0.001, bitCount: 1_437_759, hashCount: 10},
	ProfileDefault: {ratedInsertions: 1_000_000, targetFPR: 0.0001, bitCount: 19_170_117, hashCount: 13},
}

// Sketch is a mergeable Bloom filter over pre-hashed uint64 values.
type Sketch struct {
	profile   Profile
	config    profileConfig
	domain    sketchhash.Domain
	algorithm sketchhash.Algorithm

	bitset        []byte
	insertedCount uint64
}

// New constructs an empty Bloom sketch from a named profile.
func New(profile Profile, domain sketchhash.Domain, algorithm sketchhash.Algorithm) (*Sketch, error) {
	config, ok := profileConfigs[profile]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownProfile, profile)
	}
	if !sketchhash.IsRegisteredDomain(domain) {
		return nil, fmt.Errorf("%w: %s", sketchhash.ErrUnregisteredDomain, domain)
	}
	if algorithm != sketchhash.HMACSHA25664 {
		return nil, fmt.Errorf("%w: %s", ErrIncompatibleMerge, algorithm)
	}

	return newSketch(profile, config, domain, algorithm), nil
}

func newSketch(
	profile Profile,
	config profileConfig,
	domain sketchhash.Domain,
	algorithm sketchhash.Algorithm,
) *Sketch {
	return &Sketch{
		profile:   profile,
		config:    config,
		domain:    domain,
		algorithm: algorithm,
		bitset:    make([]byte, byteLen(config.bitCount)),
	}
}

// AddHash inserts a pre-hashed value.
func (s *Sketch) AddHash(hash uint64) error {
	if s.insertedCount == math.MaxUint64 {
		return ErrCountOverflow
	}
	for i := uint32(0); i < s.config.hashCount; i++ {
		s.setBit(s.location(hash, i))
	}
	s.insertedCount++

	return nil
}

// MayContainHash returns true when hash may have been inserted.
func (s *Sketch) MayContainHash(hash uint64) bool {
	for i := uint32(0); i < s.config.hashCount; i++ {
		if !s.bit(s.location(hash, i)) {
			return false
		}
	}

	return true
}

// Merge ORs other into s. Metadata must match under the v0.1 merge policy.
func (s *Sketch) Merge(other *Sketch) error {
	if other == nil {
		return nil
	}
	if s.profile != other.profile ||
		s.config.bitCount != other.config.bitCount ||
		s.config.hashCount != other.config.hashCount ||
		s.domain != other.domain ||
		s.algorithm != other.algorithm {
		return fmt.Errorf("%w: metadata differs", ErrIncompatibleMerge)
	}
	if s.insertedCount > math.MaxUint64-other.insertedCount {
		return ErrCountOverflow
	}
	for i := range s.bitset {
		s.bitset[i] |= other.bitset[i]
	}
	s.insertedCount += other.insertedCount

	return nil
}

// FalsePositiveEstimate returns the Bloom FPR estimate from the serialized
// bitset fill ratio. This keeps estimates merge-exact after OR merges.
func (s *Sketch) FalsePositiveEstimate() float64 {
	fillRatio := float64(s.SetBitCount()) / float64(s.config.bitCount)
	return math.Pow(fillRatio, float64(s.config.hashCount))
}

// InsertedCount returns the number of AddHash calls observed by this sketch.
func (s *Sketch) InsertedCount() uint64 {
	return s.insertedCount
}

// BitCount returns the configured bit count.
func (s *Sketch) BitCount() uint64 {
	return s.config.bitCount
}

// SetBitCount returns the number of one bits in the serialized bitset.
func (s *Sketch) SetBitCount() uint64 {
	var count uint64
	for _, b := range s.bitset {
		count += uint64(bits.OnesCount8(b))
	}

	return count
}

// HashCount returns the configured hash count.
func (s *Sketch) HashCount() uint32 {
	return s.config.hashCount
}

// RatedInsertions returns the profile's rated insertion count.
func (s *Sketch) RatedInsertions() uint64 {
	return s.config.ratedInsertions
}

// TargetFPR returns the profile's target false-positive rate.
func (s *Sketch) TargetFPR() float64 {
	return s.config.targetFPR
}

// RepresentationMode returns the current protobuf representation mode.
func (s *Sketch) RepresentationMode() sketchpb.RepresentationMode {
	return sketchpb.RepresentationMode_REPRESENTATION_MODE_BLOOM_BITSET
}

// Clone returns a deep copy of s.
func (s *Sketch) Clone() *Sketch {
	clone := newSketch(s.profile, s.config, s.domain, s.algorithm)
	clone.insertedCount = s.insertedCount
	copy(clone.bitset, s.bitset)

	return clone
}

// MarshalBinary returns deterministic protobuf bytes.
func (s *Sketch) MarshalBinary() ([]byte, error) {
	message := s.toProto()
	return proto.MarshalOptions{Deterministic: true}.Marshal(message)
}

// Parse decodes a Bloom sketch from deterministic protobuf bytes.
func Parse(data []byte) (*Sketch, error) {
	if len(data) > maxWireBytes {
		return nil, fmt.Errorf("%w: input length %d exceeds %d", ErrInvalidWireEncoding, len(data), maxWireBytes)
	}

	var message sketchpb.Sketch
	if err := proto.Unmarshal(data, &message); err != nil {
		return nil, err
	}

	return fromProto(&message)
}

func (s *Sketch) location(hash uint64, index uint32) uint64 {
	return hashfamily.BloomHash(hash, index) % s.config.bitCount
}

func (s *Sketch) setBit(position uint64) {
	s.bitset[position/8] |= byte(1 << (position % 8))
}

func (s *Sketch) bit(position uint64) bool {
	return s.bitset[position/8]&byte(1<<(position%8)) != 0
}

func (s *Sketch) toProto() *sketchpb.Sketch {
	bitCount := s.config.bitCount
	hashCount := s.config.hashCount
	return &sketchpb.Sketch{
		Metadata: &sketchpb.SketchMetadata{
			Kind:               sketchpb.SketchKind_SKETCH_KIND_BLOOM,
			WireVersion:        wireVersion,
			Profile:            string(s.profile),
			HashDomain:         string(s.domain),
			HashAlgo:           sketchpb.HashAlgorithm_HASH_ALGORITHM_HMAC_SHA256_64,
			RepresentationMode: s.RepresentationMode(),
			BloomBitCount:      &bitCount,
			BloomHashCount:     &hashCount,
		},
		Body: &sketchpb.Sketch_Bloom{
			Bloom: &sketchpb.BloomSketch{
				Bitset:        append([]byte(nil), s.bitset...),
				InsertedCount: s.insertedCount,
			},
		},
	}
}

func fromProto(message *sketchpb.Sketch) (*Sketch, error) {
	metadata := message.GetMetadata()
	body := message.GetBloom()
	if metadata == nil || body == nil {
		return nil, fmt.Errorf("%w: missing bloom metadata or body", ErrInvalidWireEncoding)
	}
	if metadata.GetKind() != sketchpb.SketchKind_SKETCH_KIND_BLOOM {
		return nil, fmt.Errorf("%w: kind %s", ErrInvalidWireEncoding, metadata.GetKind())
	}
	if metadata.GetWireVersion() != wireVersion {
		return nil, fmt.Errorf("%w: wire version %d", ErrInvalidWireEncoding, metadata.GetWireVersion())
	}
	if metadata.GetHashAlgo() != sketchpb.HashAlgorithm_HASH_ALGORITHM_HMAC_SHA256_64 {
		return nil, fmt.Errorf("%w: hash algorithm %s", ErrInvalidWireEncoding, metadata.GetHashAlgo())
	}
	if metadata.GetRepresentationMode() != sketchpb.RepresentationMode_REPRESENTATION_MODE_BLOOM_BITSET {
		return nil, fmt.Errorf("%w: representation %s", ErrInvalidWireEncoding, metadata.GetRepresentationMode())
	}

	profile := Profile(metadata.GetProfile())
	config, ok := profileConfigs[profile]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownProfile, profile)
	}
	if metadata.GetBloomBitCount() != config.bitCount || metadata.GetBloomHashCount() != config.hashCount {
		return nil, fmt.Errorf("%w: profile %s bit_count=%d hash_count=%d",
			ErrInvalidShape, profile, metadata.GetBloomBitCount(), metadata.GetBloomHashCount())
	}
	if !sketchhash.IsRegisteredDomain(sketchhash.Domain(metadata.GetHashDomain())) {
		return nil, fmt.Errorf("%w: %s", sketchhash.ErrUnregisteredDomain, metadata.GetHashDomain())
	}
	if len(body.GetBitset()) != int(byteLen(config.bitCount)) {
		return nil, fmt.Errorf("%w: bitset length", ErrInvalidWireEncoding)
	}
	if !finalBytePaddingZero(body.GetBitset(), config.bitCount) {
		return nil, fmt.Errorf("%w: final byte padding bits set", ErrInvalidWireEncoding)
	}

	sketch := newSketch(profile, config, sketchhash.Domain(metadata.GetHashDomain()), sketchhash.HMACSHA25664)
	sketch.insertedCount = body.GetInsertedCount()
	copy(sketch.bitset, body.GetBitset())

	return sketch, nil
}

func byteLen(bitCount uint64) uint64 {
	return (bitCount + 7) / 8
}

func finalBytePaddingZero(bitset []byte, bitCount uint64) bool {
	remainder := bitCount % 8
	if remainder == 0 || len(bitset) == 0 {
		return true
	}
	mask := byte(0xff << remainder)
	return bitset[len(bitset)-1]&mask == 0
}
