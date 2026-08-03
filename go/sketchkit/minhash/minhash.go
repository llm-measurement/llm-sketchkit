// Code authors: Vijay Erramilli and Codex
package minhash

import (
	"errors"
	"fmt"
	"math"

	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/internal/hashfamily"
	sketchpb "github.com/llm-measurement/llm-sketchkit/go/sketchkit/internal/pb"
	"google.golang.org/protobuf/proto"
)

const (
	wireVersion  = 1
	maxWireBytes = 4 << 20
)

// Profile names a MinHash profile from spec/profiles.md.
type Profile string

const (
	ProfileMicro   Profile = "micro"
	ProfileSmall   Profile = "small"
	ProfileDefault Profile = "default"
	ProfileK256    Profile = "k256"
)

var (
	// ErrUnknownProfile reports a profile outside spec/profiles.md.
	ErrUnknownProfile = errors.New("unknown minhash profile")

	// ErrInvalidSignatureLength reports a non-profile signature length.
	ErrInvalidSignatureLength = errors.New("invalid minhash signature length")

	// ErrIncompatibleMerge reports metadata mismatch on merge.
	ErrIncompatibleMerge = errors.New("incompatible minhash merge")

	// ErrCountOverflow reports populated-count overflow.
	ErrCountOverflow = errors.New("minhash populated count overflow")

	// ErrInvalidWireEncoding reports a malformed serialized sketch.
	ErrInvalidWireEncoding = errors.New("invalid minhash wire encoding")
)

var profileLengths = map[Profile]int{
	ProfileMicro:   64,
	ProfileSmall:   128,
	ProfileDefault: 128,
	ProfileK256:    256,
}

// Sketch is a mergeable MinHash signature over pre-hashed uint64 values.
type Sketch struct {
	profile   Profile
	length    int
	domain    sketchhash.Domain
	algorithm sketchhash.Algorithm

	signature      []uint64
	populatedCount uint64
}

// New constructs an empty MinHash sketch from a named profile.
func New(profile Profile, domain sketchhash.Domain, algorithm sketchhash.Algorithm) (*Sketch, error) {
	length, ok := profileLengths[profile]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownProfile, profile)
	}
	if !sketchhash.IsRegisteredDomain(domain) {
		return nil, fmt.Errorf("%w: %s", sketchhash.ErrUnregisteredDomain, domain)
	}
	if algorithm != sketchhash.HMACSHA25664 {
		return nil, fmt.Errorf("%w: %s", ErrIncompatibleMerge, algorithm)
	}

	return newSketch(profile, length, domain, algorithm), nil
}

func newSketch(
	profile Profile,
	length int,
	domain sketchhash.Domain,
	algorithm sketchhash.Algorithm,
) *Sketch {
	signature := make([]uint64, length)
	for i := range signature {
		signature[i] = math.MaxUint64
	}

	return &Sketch{
		profile:   profile,
		length:    length,
		domain:    domain,
		algorithm: algorithm,
		signature: signature,
	}
}

// AddHash updates the signature with a pre-hashed value.
func (s *Sketch) AddHash(hash uint64) error {
	if s.populatedCount == math.MaxUint64 {
		return ErrCountOverflow
	}
	for i := range s.signature {
		value := hashfamily.MinHash(hash, uint32(i))
		if value < s.signature[i] {
			s.signature[i] = value
		}
	}
	s.populatedCount++

	return nil
}

// Merge applies element-wise minimum with other. Metadata must match.
func (s *Sketch) Merge(other *Sketch) error {
	if other == nil {
		return nil
	}
	if s.profile != other.profile ||
		s.length != other.length ||
		s.domain != other.domain ||
		s.algorithm != other.algorithm {
		return fmt.Errorf("%w: metadata differs", ErrIncompatibleMerge)
	}
	if s.populatedCount > math.MaxUint64-other.populatedCount {
		return ErrCountOverflow
	}
	for i, value := range other.signature {
		if value < s.signature[i] {
			s.signature[i] = value
		}
	}
	s.populatedCount += other.populatedCount

	return nil
}

// JaccardEstimate returns the MinHash Jaccard estimate against other.
func (s *Sketch) JaccardEstimate(other *Sketch) (float64, error) {
	if other == nil {
		return 0, fmt.Errorf("%w: nil", ErrIncompatibleMerge)
	}
	if s.profile != other.profile ||
		s.length != other.length ||
		s.domain != other.domain ||
		s.algorithm != other.algorithm {
		return 0, fmt.Errorf("%w: metadata differs", ErrIncompatibleMerge)
	}
	if s.populatedCount == 0 && other.populatedCount == 0 {
		return 1, nil
	}
	if s.populatedCount == 0 || other.populatedCount == 0 {
		return 0, nil
	}

	matches := 0
	for i, value := range s.signature {
		if value == other.signature[i] {
			matches++
		}
	}

	return float64(matches) / float64(s.length), nil
}

// PopulatedCount returns the number of AddHash calls observed by this sketch.
func (s *Sketch) PopulatedCount() uint64 {
	return s.populatedCount
}

// SignatureLength returns the configured signature length.
func (s *Sketch) SignatureLength() int {
	return s.length
}

// Signature returns a copy of the current signature.
func (s *Sketch) Signature() []uint64 {
	out := make([]uint64, len(s.signature))
	copy(out, s.signature)

	return out
}

// RepresentationMode returns the current protobuf representation mode.
func (s *Sketch) RepresentationMode() sketchpb.RepresentationMode {
	return sketchpb.RepresentationMode_REPRESENTATION_MODE_MINHASH_SIGNATURE
}

// Clone returns a deep copy of s.
func (s *Sketch) Clone() *Sketch {
	clone := newSketch(s.profile, s.length, s.domain, s.algorithm)
	clone.populatedCount = s.populatedCount
	copy(clone.signature, s.signature)

	return clone
}

// MarshalBinary returns deterministic protobuf bytes.
func (s *Sketch) MarshalBinary() ([]byte, error) {
	message := s.toProto()
	return proto.MarshalOptions{Deterministic: true}.Marshal(message)
}

// Parse decodes a MinHash sketch from deterministic protobuf bytes.
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

func (s *Sketch) toProto() *sketchpb.Sketch {
	length := uint32(s.length)
	return &sketchpb.Sketch{
		Metadata: &sketchpb.SketchMetadata{
			Kind:                   sketchpb.SketchKind_SKETCH_KIND_MINHASH,
			WireVersion:            wireVersion,
			Profile:                string(s.profile),
			HashDomain:             string(s.domain),
			HashAlgo:               sketchpb.HashAlgorithm_HASH_ALGORITHM_HMAC_SHA256_64,
			RepresentationMode:     s.RepresentationMode(),
			MinhashSignatureLength: &length,
		},
		Body: &sketchpb.Sketch_Minhash{
			Minhash: &sketchpb.MinHashSketch{
				Signature:      append([]uint64(nil), s.signature...),
				PopulatedCount: s.populatedCount,
			},
		},
	}
}

func fromProto(message *sketchpb.Sketch) (*Sketch, error) {
	metadata := message.GetMetadata()
	body := message.GetMinhash()
	if metadata == nil || body == nil {
		return nil, fmt.Errorf("%w: missing minhash metadata or body", ErrInvalidWireEncoding)
	}
	if metadata.GetKind() != sketchpb.SketchKind_SKETCH_KIND_MINHASH {
		return nil, fmt.Errorf("%w: kind %s", ErrInvalidWireEncoding, metadata.GetKind())
	}
	if metadata.GetWireVersion() != wireVersion {
		return nil, fmt.Errorf("%w: wire version %d", ErrInvalidWireEncoding, metadata.GetWireVersion())
	}
	if metadata.GetHashAlgo() != sketchpb.HashAlgorithm_HASH_ALGORITHM_HMAC_SHA256_64 {
		return nil, fmt.Errorf("%w: hash algorithm %s", ErrInvalidWireEncoding, metadata.GetHashAlgo())
	}
	if metadata.GetRepresentationMode() != sketchpb.RepresentationMode_REPRESENTATION_MODE_MINHASH_SIGNATURE {
		return nil, fmt.Errorf("%w: representation %s", ErrInvalidWireEncoding, metadata.GetRepresentationMode())
	}

	profile := Profile(metadata.GetProfile())
	length, ok := profileLengths[profile]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownProfile, profile)
	}
	if metadata.GetMinhashSignatureLength() != uint32(length) {
		return nil, fmt.Errorf("%w: profile %s length=%d",
			ErrInvalidSignatureLength, profile, metadata.GetMinhashSignatureLength())
	}
	if !sketchhash.IsRegisteredDomain(sketchhash.Domain(metadata.GetHashDomain())) {
		return nil, fmt.Errorf("%w: %s", sketchhash.ErrUnregisteredDomain, metadata.GetHashDomain())
	}
	if len(body.GetSignature()) != length {
		return nil, fmt.Errorf("%w: signature length", ErrInvalidWireEncoding)
	}
	if body.GetPopulatedCount() == 0 {
		for _, value := range body.GetSignature() {
			if value != math.MaxUint64 {
				return nil, fmt.Errorf("%w: non-empty signature with zero count", ErrInvalidWireEncoding)
			}
		}
	}

	sketch := newSketch(profile, length, sketchhash.Domain(metadata.GetHashDomain()), sketchhash.HMACSHA25664)
	sketch.populatedCount = body.GetPopulatedCount()
	copy(sketch.signature, body.GetSignature())

	return sketch, nil
}
