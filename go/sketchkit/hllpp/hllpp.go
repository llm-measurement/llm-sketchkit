// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay Erramilli and Codex
package hllpp

import (
	"errors"
	"fmt"
	"math"
	"math/bits"
	"sort"

	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
	sketchpb "github.com/llm-measurement/llm-sketchkit/go/sketchkit/internal/pb"
	"google.golang.org/protobuf/proto"
)

const (
	wireVersion  = 1
	maxWireBytes = 4 << 20

	// Wire bytes use one byte per dense register. HLL++ ranks fit in 6 bits for
	// the supported 64-bit hash path, so this stays simple and deterministic.
	denseRegisterBytes = 1
)

// Profile names a HLL++ profile from spec/profiles.md.
type Profile string

const (
	ProfileMicro   Profile = "micro"
	ProfileSmall   Profile = "small"
	ProfileDefault Profile = "default"
)

var (
	// ErrUnknownProfile reports a profile outside spec/profiles.md.
	ErrUnknownProfile = errors.New("unknown hllpp profile")

	// ErrInvalidPrecision reports an invalid HLL++ precision pair.
	ErrInvalidPrecision = errors.New("invalid hllpp precision")

	// ErrIncompatibleMerge reports non-precision metadata mismatches on merge.
	ErrIncompatibleMerge = errors.New("incompatible hllpp merge")

	// ErrPrecisionMismatch reports normal or sparse precision mismatch on merge.
	ErrPrecisionMismatch = errors.New("hllpp precision mismatch")

	// ErrInvalidWireEncoding reports a malformed serialized sketch.
	ErrInvalidWireEncoding = errors.New("invalid hllpp wire encoding")
)

type profileConfig struct {
	p                  uint8
	sp                 uint8
	promotionThreshold int
}

var profileConfigs = map[Profile]profileConfig{
	ProfileMicro:   {p: 12, sp: 16, promotionThreshold: 1 << (12 - 4)},
	ProfileSmall:   {p: 14, sp: 18, promotionThreshold: 1 << (14 - 4)},
	ProfileDefault: {p: 15, sp: 20, promotionThreshold: 1 << (15 - 4)},
}

// Sketch is a mergeable HLL++ sketch over pre-hashed uint64 values.
type Sketch struct {
	profile            Profile
	p                  uint8
	sp                 uint8
	domain             sketchhash.Domain
	algorithm          sketchhash.Algorithm
	promotionThreshold int

	sparse *sparseRegisters
	dense  []uint8
}

type sparseRegisters struct {
	indexes []uint32
	ranks   []uint8
}

// New constructs an empty HLL++ sketch from a named profile.
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

	return newSketch(profile, config.p, config.sp, domain, algorithm, config.promotionThreshold)
}

func newSketch(
	profile Profile,
	p uint8,
	sp uint8,
	domain sketchhash.Domain,
	algorithm sketchhash.Algorithm,
	promotionThreshold int,
) (*Sketch, error) {
	config, ok := profileConfigs[profile]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownProfile, profile)
	}
	if p != config.p || sp != config.sp {
		return nil, fmt.Errorf("%w: profile %s carries p/sp %d/%d", ErrPrecisionMismatch, profile, p, sp)
	}
	if err := validatePrecision(p, sp); err != nil {
		return nil, err
	}
	if promotionThreshold <= 0 {
		return nil, fmt.Errorf("%w: promotion threshold must be positive", ErrInvalidPrecision)
	}

	return &Sketch{
		profile:            profile,
		p:                  p,
		sp:                 sp,
		domain:             domain,
		algorithm:          algorithm,
		promotionThreshold: promotionThreshold,
		sparse:             newSparseRegisters(0),
	}, nil
}

// AddHash updates the sketch with a pre-hashed uint64.
// It has no recoverable error return; runtime panics have no rollback guarantee.
func (s *Sketch) AddHash(hash uint64) {
	if s.sparse != nil {
		index, rank := sparseRegister(hash, s.sp)
		s.sparse.Update(index, rank)
		if s.sparse.Len() > s.promotionThreshold {
			s.promote()
		}
		return
	}

	index, rank := denseRegister(hash, s.p)
	if rank > s.dense[index] {
		s.dense[index] = rank
	}
}

// Merge adds other into s. Metadata must match under the v0.1 merge policy.
// A returned error leaves s unchanged. A nil source is a no-op, and a distinct
// source is never modified. This does not cover runtime panics or concurrent use.
func (s *Sketch) Merge(other *Sketch) error {
	if other == nil {
		return nil
	}
	if s.p != other.p || s.sp != other.sp {
		return fmt.Errorf("%w: p/sp %d/%d vs %d/%d", ErrPrecisionMismatch, s.p, s.sp, other.p, other.sp)
	}
	if s.profile != other.profile || s.domain != other.domain || s.algorithm != other.algorithm {
		return fmt.Errorf("%w: metadata differs", ErrIncompatibleMerge)
	}

	switch {
	case s.sparse != nil && other.sparse != nil:
		other.sparse.Range(func(index uint32, rank uint8) {
			s.sparse.Update(index, rank)
		})
		if s.sparse.Len() > s.promotionThreshold {
			s.promote()
		}
	case s.sparse != nil && other.sparse == nil:
		s.promote()
		mergeDense(s.dense, other.dense)
	case s.sparse == nil && other.sparse != nil:
		other.sparse.Range(func(index uint32, rank uint8) {
			denseIndex, denseRank := sparseToDenseRegister(index, rank, s.p, s.sp)
			if denseRank > s.dense[denseIndex] {
				s.dense[denseIndex] = denseRank
			}
		})
	default:
		mergeDense(s.dense, other.dense)
	}

	return nil
}

// Estimate returns the cardinality estimate.
func (s *Sketch) Estimate() float64 {
	if s.sparse != nil {
		return linearCounting(uint64(1)<<s.sp, uint64(s.sparse.Len()))
	}

	return s.denseEstimate()
}

// RepresentationMode returns the current protobuf representation mode.
func (s *Sketch) RepresentationMode() sketchpb.RepresentationMode {
	if s.sparse != nil {
		return sketchpb.RepresentationMode_REPRESENTATION_MODE_HLLPP_SPARSE
	}

	return sketchpb.RepresentationMode_REPRESENTATION_MODE_HLLPP_DENSE
}

// SparseCount returns the number of sparse registers currently stored.
func (s *Sketch) SparseCount() int {
	if s.sparse == nil {
		return 0
	}

	return s.sparse.Len()
}

// DenseNonZeroCount returns the number of non-zero dense registers.
func (s *Sketch) DenseNonZeroCount() int {
	if s.dense == nil {
		return 0
	}

	count := 0
	for _, rank := range s.dense {
		if rank != 0 {
			count++
		}
	}

	return count
}

// DenseRegisterBytes returns the fixed dense register byte count for this sketch.
func (s *Sketch) DenseRegisterBytes() int {
	return denseRegisterBytes * (1 << s.p)
}

// RegisterStorageBytes returns the logical register-storage bytes for the
// current representation, excluding protobuf metadata and allocator headers.
func (s *Sketch) RegisterStorageBytes() int {
	if s.sparse != nil {
		return s.sparse.StorageBytes()
	}

	return s.DenseRegisterBytes()
}

// ForceDense promotes a sparse sketch to dense representation.
// It has no recoverable error return; runtime panics have no rollback guarantee.
func (s *Sketch) ForceDense() {
	if s.sparse != nil {
		s.promote()
	}
}

// Clone returns a deep copy of s.
func (s *Sketch) Clone() *Sketch {
	clone := &Sketch{
		profile:            s.profile,
		p:                  s.p,
		sp:                 s.sp,
		domain:             s.domain,
		algorithm:          s.algorithm,
		promotionThreshold: s.promotionThreshold,
	}
	if s.sparse != nil {
		clone.sparse = s.sparse.Clone()
	} else {
		clone.dense = append([]uint8(nil), s.dense...)
	}

	return clone
}

// MarshalBinary returns deterministic protobuf bytes.
func (s *Sketch) MarshalBinary() ([]byte, error) {
	message := s.toProto()
	return proto.MarshalOptions{Deterministic: true}.Marshal(message)
}

// Parse decodes a HLL++ sketch from deterministic protobuf bytes.
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
	normalPrecision := uint32(s.p)
	sparsePrecision := uint32(s.sp)
	metadata := &sketchpb.SketchMetadata{
		Kind:                 sketchpb.SketchKind_SKETCH_KIND_HLLPP,
		WireVersion:          wireVersion,
		Profile:              string(s.profile),
		HashDomain:           string(s.domain),
		HashAlgo:             sketchpb.HashAlgorithm_HASH_ALGORITHM_HMAC_SHA256_64,
		RepresentationMode:   s.RepresentationMode(),
		HllppNormalPrecision: &normalPrecision,
		HllppSparsePrecision: &sparsePrecision,
	}

	body := &sketchpb.HllppSketch{}
	if s.sparse != nil {
		body.SparseRegisters = make([]*sketchpb.HllppSparseRegister, 0, s.sparse.Len())
		s.sparse.Range(func(index uint32, rank uint8) {
			body.SparseRegisters = append(body.SparseRegisters, &sketchpb.HllppSparseRegister{
				Index: index,
				Value: uint32(rank),
			})
		})
	} else {
		body.DenseRegisters = append([]byte(nil), s.dense...)
	}

	return &sketchpb.Sketch{
		Metadata: metadata,
		Body:     &sketchpb.Sketch_Hllpp{Hllpp: body},
	}
}

func fromProto(message *sketchpb.Sketch) (*Sketch, error) {
	metadata := message.GetMetadata()
	body := message.GetHllpp()
	if metadata == nil || body == nil {
		return nil, fmt.Errorf("%w: missing HLL++ metadata or body", ErrInvalidWireEncoding)
	}
	if metadata.GetKind() != sketchpb.SketchKind_SKETCH_KIND_HLLPP {
		return nil, fmt.Errorf("%w: kind %s", ErrInvalidWireEncoding, metadata.GetKind())
	}
	if metadata.GetWireVersion() != wireVersion {
		return nil, fmt.Errorf("%w: wire version %d", ErrInvalidWireEncoding, metadata.GetWireVersion())
	}
	if metadata.GetHashAlgo() != sketchpb.HashAlgorithm_HASH_ALGORITHM_HMAC_SHA256_64 {
		return nil, fmt.Errorf("%w: hash algorithm %s", ErrInvalidWireEncoding, metadata.GetHashAlgo())
	}

	profile := Profile(metadata.GetProfile())
	config, ok := profileConfigs[profile]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownProfile, profile)
	}
	domain := sketchhash.Domain(metadata.GetHashDomain())
	if !sketchhash.IsRegisteredDomain(domain) {
		return nil, fmt.Errorf("%w: %s", sketchhash.ErrUnregisteredDomain, domain)
	}
	normalPrecisionRaw := metadata.GetHllppNormalPrecision()
	sparsePrecisionRaw := metadata.GetHllppSparsePrecision()
	if normalPrecisionRaw != uint32(config.p) || sparsePrecisionRaw != uint32(config.sp) {
		return nil, fmt.Errorf("%w: profile %s carries p/sp %d/%d", ErrPrecisionMismatch, profile, normalPrecisionRaw, sparsePrecisionRaw)
	}
	normalPrecision := config.p
	sparsePrecision := config.sp
	sketch, err := newSketch(
		profile,
		normalPrecision,
		sparsePrecision,
		domain,
		sketchhash.HMACSHA25664,
		config.promotionThreshold,
	)
	if err != nil {
		return nil, err
	}

	switch metadata.GetRepresentationMode() {
	case sketchpb.RepresentationMode_REPRESENTATION_MODE_HLLPP_SPARSE:
		registers := body.GetSparseRegisters()
		if len(registers) > config.promotionThreshold {
			return nil, fmt.Errorf("%w: sparse register count %d exceeds %d", ErrInvalidWireEncoding, len(registers), config.promotionThreshold)
		}
		maxSparseIndex := uint32(1) << sparsePrecision
		sketch.sparse = newSparseRegisters(len(registers))
		var previous uint32
		for i, register := range registers {
			index := register.GetIndex()
			value := register.GetValue()
			if i > 0 && index <= previous {
				return nil, fmt.Errorf("%w: sparse registers unsorted", ErrInvalidWireEncoding)
			}
			if index >= maxSparseIndex {
				return nil, fmt.Errorf("%w: sparse index %d", ErrInvalidWireEncoding, index)
			}
			if value == 0 || value > uint32(64-sparsePrecision+1) {
				return nil, fmt.Errorf("%w: sparse rank %d", ErrInvalidWireEncoding, value)
			}
			sketch.sparse.Update(index, uint8(value))
			previous = index
		}
		if len(body.GetDenseRegisters()) != 0 {
			return nil, fmt.Errorf("%w: sparse body contains dense bytes", ErrInvalidWireEncoding)
		}
	case sketchpb.RepresentationMode_REPRESENTATION_MODE_HLLPP_DENSE:
		expected := 1 << normalPrecision
		if len(body.GetDenseRegisters()) != expected {
			return nil, fmt.Errorf("%w: dense length %d, want %d", ErrInvalidWireEncoding, len(body.GetDenseRegisters()), expected)
		}
		if len(body.GetSparseRegisters()) != 0 {
			return nil, fmt.Errorf("%w: dense body contains sparse registers", ErrInvalidWireEncoding)
		}
		maxDenseRank := uint8(64 - normalPrecision + 1)
		for i, rank := range body.GetDenseRegisters() {
			if rank > maxDenseRank {
				return nil, fmt.Errorf("%w: dense rank %d at index %d", ErrInvalidWireEncoding, rank, i)
			}
		}
		sketch.sparse = nil
		sketch.dense = append([]uint8(nil), body.GetDenseRegisters()...)
	default:
		return nil, fmt.Errorf("%w: representation %s", ErrInvalidWireEncoding, metadata.GetRepresentationMode())
	}

	return sketch, nil
}

func (s *Sketch) denseEstimate() float64 {
	m := 1 << s.p
	sum := 0.0
	zeroCount := 0
	for _, rank := range s.dense {
		sum += math.Ldexp(1, -int(rank))
		if rank == 0 {
			zeroCount++
		}
	}

	raw := alpha(uint64(m)) * float64(m*m) / sum
	estimatePrime := raw
	if raw <= 5*float64(m) {
		estimatePrime = raw - estimateBias(raw, s.p)
	}

	if zeroCount != 0 {
		lc := linearCounting(uint64(m), uint64(m-zeroCount))
		if lc <= linearCountingThreshold(s.p) {
			return lc
		}
	}

	return estimatePrime
}

func (s *Sketch) promote() {
	dense := make([]uint8, 1<<s.p)
	s.sparse.Range(func(index uint32, rank uint8) {
		denseIndex, denseRank := sparseToDenseRegister(index, rank, s.p, s.sp)
		if denseRank > dense[denseIndex] {
			dense[denseIndex] = denseRank
		}
	})

	s.sparse = nil
	s.dense = dense
}

func newSparseRegisters(capacity int) *sparseRegisters {
	return &sparseRegisters{
		indexes: make([]uint32, 0, capacity),
		ranks:   make([]uint8, 0, capacity),
	}
}

func (r *sparseRegisters) Len() int {
	return len(r.indexes)
}

func (r *sparseRegisters) Get(index uint32) (uint8, bool) {
	position, ok := r.search(index)
	if !ok {
		return 0, false
	}

	return r.ranks[position], true
}

func (r *sparseRegisters) Update(index uint32, rank uint8) {
	position, ok := r.search(index)
	if ok {
		if rank > r.ranks[position] {
			r.ranks[position] = rank
		}
		return
	}

	r.indexes = append(r.indexes, 0)
	copy(r.indexes[position+1:], r.indexes[position:])
	r.indexes[position] = index

	r.ranks = append(r.ranks, 0)
	copy(r.ranks[position+1:], r.ranks[position:])
	r.ranks[position] = rank
}

func (r *sparseRegisters) Range(yield func(index uint32, rank uint8)) {
	for i, index := range r.indexes {
		yield(index, r.ranks[i])
	}
}

func (r *sparseRegisters) Clone() *sparseRegisters {
	return &sparseRegisters{
		indexes: append([]uint32(nil), r.indexes...),
		ranks:   append([]uint8(nil), r.ranks...),
	}
}

func (r *sparseRegisters) StorageBytes() int {
	return cap(r.indexes)*4 + cap(r.ranks)
}

func (r *sparseRegisters) search(index uint32) (int, bool) {
	position := sort.Search(len(r.indexes), func(i int) bool {
		return r.indexes[i] >= index
	})
	if position < len(r.indexes) && r.indexes[position] == index {
		return position, true
	}

	return position, false
}

func mergeDense(dst []uint8, src []uint8) {
	for i, rank := range src {
		if rank > dst[i] {
			dst[i] = rank
		}
	}
}

func validatePrecision(p uint8, sp uint8) error {
	if p < 4 || p > 25 {
		return fmt.Errorf("%w: normal precision %d", ErrInvalidPrecision, p)
	}
	if sp < p || sp > 32 {
		return fmt.Errorf("%w: sparse precision %d for p %d", ErrInvalidPrecision, sp, p)
	}
	return nil
}

func denseRegister(hash uint64, p uint8) (uint32, uint8) {
	index := uint32(hash >> (64 - p))
	rank := registerRank(hash<<p, 64-int(p))

	return index, rank
}

func sparseRegister(hash uint64, sp uint8) (uint32, uint8) {
	index := uint32(hash >> (64 - sp))
	rank := registerRank(hash<<sp, 64-int(sp))

	return index, rank
}

func sparseToDenseRegister(sparseIndex uint32, sparseRank uint8, p uint8, sp uint8) (uint32, uint8) {
	extraBits := sp - p
	denseIndex := sparseIndex >> extraBits
	extraMask := uint32((1 << extraBits) - 1)
	extra := sparseIndex & extraMask
	if extra != 0 {
		shift := 64 - extraBits
		return denseIndex, registerRank(uint64(extra)<<shift, int(extraBits))
	}

	return denseIndex, uint8(extraBits) + sparseRank
}

func registerRank(shifted uint64, width int) uint8 {
	rank := bits.LeadingZeros64(shifted) + 1
	maxRank := width + 1
	if rank > maxRank {
		rank = maxRank
	}

	return uint8(rank)
}

func linearCounting(registerCount uint64, occupied uint64) float64 {
	if occupied == 0 {
		return 0
	}
	if occupied >= registerCount {
		return float64(registerCount)
	}

	return float64(registerCount) * math.Log(float64(registerCount)/float64(registerCount-occupied))
}

func alpha(m uint64) float64 {
	switch m {
	case 16:
		return 0.673
	case 32:
		return 0.697
	case 64:
		return 0.709
	default:
		return 0.7213 / (1 + 1.079/float64(m))
	}
}

func linearCountingThreshold(p uint8) float64 {
	switch p {
	case 4:
		return 10
	case 5:
		return 20
	case 6:
		return 40
	case 7:
		return 80
	case 8:
		return 220
	case 9:
		return 400
	case 10:
		return 900
	case 11:
		return 1800
	case 12:
		return 3100
	case 13:
		return 6500
	case 14:
		return 11500
	case 15:
		return 20000
	case 16:
		return 50000
	case 17:
		return 120000
	case 18:
		return 350000
	default:
		threshold := 5 * float64(uint64(1)<<p) / 2
		return threshold
	}
}

func estimateBias(estimate float64, p uint8) float64 {
	estimates := rawEstimateData[p]
	biases := biasData[p]
	if len(estimates) == 0 || len(estimates) != len(biases) {
		return 0
	}

	indexes := make([]int, len(estimates))
	for i := range indexes {
		indexes[i] = i
	}
	sort.Slice(indexes, func(i int, j int) bool {
		left := estimate - estimates[indexes[i]]
		right := estimate - estimates[indexes[j]]
		return left*left < right*right
	})

	neighbors := 6
	if len(indexes) < neighbors {
		neighbors = len(indexes)
	}
	total := 0.0
	for _, index := range indexes[:neighbors] {
		total += biases[index]
	}

	return total / float64(neighbors)
}
