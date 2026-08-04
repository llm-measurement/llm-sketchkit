// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay Erramilli and Codex
package frequentitems

import (
	"errors"
	"fmt"
	"math"
	"sort"

	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
	sketchpb "github.com/llm-measurement/llm-sketchkit/go/sketchkit/internal/pb"
	"google.golang.org/protobuf/proto"
)

const (
	wireVersion  = 1
	maxWireBytes = 4 << 20
)

// Profile names a weighted frequent-items profile from spec/profiles.md.
type Profile string

const (
	ProfileMicro   Profile = "micro"
	ProfileSmall   Profile = "small"
	ProfileDefault Profile = "default"
)

// QueryMode selects the frequent-items false-positive/false-negative contract.
type QueryMode int

const (
	NoFalsePositives QueryMode = iota + 1
	NoFalseNegatives
)

var (
	// ErrUnknownProfile reports a profile outside spec/profiles.md.
	ErrUnknownProfile = errors.New("unknown frequent-items profile")

	// ErrInvalidMapSize reports a non-profile or invalid bounded-map size.
	ErrInvalidMapSize = errors.New("invalid frequent-items map size")

	// ErrNegativeWeight reports a negative update weight.
	ErrNegativeWeight = errors.New("negative frequent-items weight")

	// ErrWeightOverflow reports an int64 weight or counter overflow.
	ErrWeightOverflow = errors.New("frequent-items weight overflow")

	// ErrIncompatibleMerge reports metadata mismatch on merge.
	ErrIncompatibleMerge = errors.New("incompatible frequent-items merge")

	// ErrInvalidQueryMode reports an unknown frequent-items query mode.
	ErrInvalidQueryMode = errors.New("invalid frequent-items query mode")

	// ErrInvalidWireEncoding reports a malformed serialized sketch.
	ErrInvalidWireEncoding = errors.New("invalid frequent-items wire encoding")
)

var profileMapSizes = map[Profile]int{
	ProfileMicro:   256,
	ProfileSmall:   512,
	ProfileDefault: 1024,
}

// Item is a returned frequent item with deterministic bounds.
type Item struct {
	Hash       uint64
	Estimate   int64
	LowerBound int64
	UpperBound int64
	Error      int64
}

type counter struct {
	hash      uint64
	estimate  int64
	heapIndex int
}

// Sketch is a weighted, mergeable frequent-items sketch over pre-hashed keys.
type Sketch struct {
	profile   Profile
	mapSize   int
	domain    sketchhash.Domain
	algorithm sketchhash.Algorithm

	totalWeight int64
	maxError    int64

	items map[uint64]*counter
	heap  []*counter
	pool  []counter
	free  []*counter
}

// New constructs an empty weighted frequent-items sketch from a named profile.
func New(profile Profile, domain sketchhash.Domain, algorithm sketchhash.Algorithm) (*Sketch, error) {
	mapSize, ok := profileMapSizes[profile]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownProfile, profile)
	}
	if !sketchhash.IsRegisteredDomain(domain) {
		return nil, fmt.Errorf("%w: %s", sketchhash.ErrUnregisteredDomain, domain)
	}
	if algorithm != sketchhash.HMACSHA25664 {
		return nil, fmt.Errorf("%w: %s", ErrIncompatibleMerge, algorithm)
	}

	return newSketch(profile, mapSize, domain, algorithm)
}

func newSketch(
	profile Profile,
	mapSize int,
	domain sketchhash.Domain,
	algorithm sketchhash.Algorithm,
) (*Sketch, error) {
	profileMapSize, ok := profileMapSizes[profile]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownProfile, profile)
	}
	if mapSize <= 0 || mapSize != profileMapSize {
		return nil, fmt.Errorf("%w: profile %s carries map size %d", ErrInvalidMapSize, profile, mapSize)
	}
	if !sketchhash.IsRegisteredDomain(domain) {
		return nil, fmt.Errorf("%w: %s", sketchhash.ErrUnregisteredDomain, domain)
	}
	if algorithm != sketchhash.HMACSHA25664 {
		return nil, fmt.Errorf("%w: %s", ErrIncompatibleMerge, algorithm)
	}

	pool := make([]counter, mapSize)
	free := make([]*counter, mapSize)
	for i := range pool {
		free[i] = &pool[i]
	}

	return &Sketch{
		profile:   profile,
		mapSize:   mapSize,
		domain:    domain,
		algorithm: algorithm,
		items:     make(map[uint64]*counter, 2*mapSize),
		heap:      make([]*counter, 0, mapSize),
		pool:      pool,
		free:      free,
	}, nil
}

// AddHash updates the sketch with a pre-hashed key and non-negative weight.
func (s *Sketch) AddHash(hash uint64, weight int64) error {
	if weight < 0 {
		return fmt.Errorf("%w: %d", ErrNegativeWeight, weight)
	}
	if weight == 0 {
		return nil
	}
	if s.totalWeight > math.MaxInt64-weight {
		return fmt.Errorf("%w: total weight", ErrWeightOverflow)
	}
	s.totalWeight += weight

	return s.addResidual(hash, weight)
}

// Merge adds other into s. Metadata must match under the v0.1 merge policy.
func (s *Sketch) Merge(other *Sketch) error {
	if other == nil {
		return nil
	}
	if s.profile != other.profile ||
		s.mapSize != other.mapSize ||
		s.domain != other.domain ||
		s.algorithm != other.algorithm {
		return fmt.Errorf("%w: metadata differs", ErrIncompatibleMerge)
	}
	if s.totalWeight > math.MaxInt64-other.totalWeight {
		return fmt.Errorf("%w: total weight", ErrWeightOverflow)
	}
	if s.maxError > math.MaxInt64-other.maxError {
		return fmt.Errorf("%w: max error", ErrWeightOverflow)
	}

	type residualEntry struct {
		hash     uint64
		residual int64
	}
	combined := make(map[uint64]int64, len(s.items)+len(other.items))
	for _, current := range s.items {
		addCombinedResidual(combined, current.hash, current.estimate-s.maxError)
	}
	for _, current := range other.items {
		addCombinedResidual(combined, current.hash, current.estimate-other.maxError)
	}

	entries := make([]residualEntry, 0, len(combined))
	for hash, residual := range combined {
		if residual > 0 {
			entries = append(entries, residualEntry{hash: hash, residual: residual})
		}
	}
	sort.Slice(entries, func(i int, j int) bool {
		return entries[i].hash < entries[j].hash
	})

	totalWeight := s.totalWeight + other.totalWeight
	maxError := s.maxError + other.maxError
	s.reset()
	s.totalWeight = totalWeight
	s.maxError = maxError
	for _, entry := range entries {
		if err := s.addResidual(entry.hash, entry.residual); err != nil {
			return err
		}
	}

	return nil
}

// EstimateHash returns the current upper estimate for a tracked hash, or zero
// if the hash is untracked. Use LowerBoundHash and UpperBoundHash for intervals.
func (s *Sketch) EstimateHash(hash uint64) int64 {
	current, ok := s.items[hash]
	if !ok {
		return 0
	}

	return current.estimate
}

// LowerBoundHash returns the deterministic lower bound for hash.
func (s *Sketch) LowerBoundHash(hash uint64) int64 {
	current, ok := s.items[hash]
	if !ok {
		return 0
	}

	return maxInt64(0, current.estimate-s.maxError)
}

// UpperBoundHash returns the deterministic upper bound for hash.
func (s *Sketch) UpperBoundHash(hash uint64) int64 {
	current, ok := s.items[hash]
	if !ok {
		return s.maxError
	}

	return current.estimate
}

// FrequentItems returns deterministic frequent items using max_error as the
// implicit threshold.
func (s *Sketch) FrequentItems(mode QueryMode) ([]Item, error) {
	items := s.itemsSortedByHash()
	out := make([]Item, 0, len(items))
	for _, item := range items {
		switch mode {
		case NoFalseNegatives:
			if item.UpperBound > s.maxError {
				out = append(out, item)
			}
		case NoFalsePositives:
			if item.LowerBound > s.maxError {
				out = append(out, item)
			}
		default:
			return nil, fmt.Errorf("%w: %d", ErrInvalidQueryMode, mode)
		}
	}
	sort.Slice(out, func(i int, j int) bool {
		if out[i].Estimate != out[j].Estimate {
			return out[i].Estimate > out[j].Estimate
		}
		return out[i].Hash < out[j].Hash
	})

	return out, nil
}

// TotalWeight returns the total non-negative update weight observed.
func (s *Sketch) TotalWeight() int64 {
	return s.totalWeight
}

// MaxError returns the global deterministic error bound.
func (s *Sketch) MaxError() int64 {
	return s.maxError
}

// MapSize returns the configured tracked-item bound.
func (s *Sketch) MapSize() int {
	return s.mapSize
}

// Len returns the current number of tracked items.
func (s *Sketch) Len() int {
	return len(s.items)
}

// RepresentationMode returns the current protobuf representation mode.
func (s *Sketch) RepresentationMode() sketchpb.RepresentationMode {
	return sketchpb.RepresentationMode_REPRESENTATION_MODE_FREQUENT_ITEMS_BOUNDED_MAP
}

// Clone returns a deep copy of s.
func (s *Sketch) Clone() *Sketch {
	clone, err := newSketch(s.profile, s.mapSize, s.domain, s.algorithm)
	if err != nil {
		panic(err)
	}
	clone.totalWeight = s.totalWeight
	clone.maxError = s.maxError
	for _, item := range s.itemsSortedByHash() {
		clone.insertCounter(item.Hash, item.Estimate)
	}

	return clone
}

// MarshalBinary returns deterministic protobuf bytes.
func (s *Sketch) MarshalBinary() ([]byte, error) {
	message := s.toProto()
	return proto.MarshalOptions{Deterministic: true}.Marshal(message)
}

// Parse decodes a weighted frequent-items sketch from deterministic protobuf bytes.
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
	mapSize := uint32(s.mapSize)
	metadata := &sketchpb.SketchMetadata{
		Kind:                 sketchpb.SketchKind_SKETCH_KIND_FREQUENT_ITEMS,
		WireVersion:          wireVersion,
		Profile:              string(s.profile),
		HashDomain:           string(s.domain),
		HashAlgo:             sketchpb.HashAlgorithm_HASH_ALGORITHM_HMAC_SHA256_64,
		RepresentationMode:   s.RepresentationMode(),
		FrequentItemsMapSize: &mapSize,
	}

	items := s.itemsSortedByHash()
	body := &sketchpb.FrequentItemsSketch{
		Entries:     make([]*sketchpb.FrequentItemsEntry, 0, len(items)),
		TotalWeight: s.totalWeight,
		MaxError:    s.maxError,
	}
	for _, item := range items {
		body.Entries = append(body.Entries, &sketchpb.FrequentItemsEntry{
			Hash:     item.Hash,
			Estimate: item.Estimate,
			Error:    item.Error,
		})
	}

	return &sketchpb.Sketch{
		Metadata: metadata,
		Body:     &sketchpb.Sketch_FrequentItems{FrequentItems: body},
	}
}

func fromProto(message *sketchpb.Sketch) (*Sketch, error) {
	metadata := message.GetMetadata()
	body := message.GetFrequentItems()
	if metadata == nil || body == nil {
		return nil, fmt.Errorf("%w: missing frequent-items metadata or body", ErrInvalidWireEncoding)
	}
	if metadata.GetKind() != sketchpb.SketchKind_SKETCH_KIND_FREQUENT_ITEMS {
		return nil, fmt.Errorf("%w: kind %s", ErrInvalidWireEncoding, metadata.GetKind())
	}
	if metadata.GetWireVersion() != wireVersion {
		return nil, fmt.Errorf("%w: wire version %d", ErrInvalidWireEncoding, metadata.GetWireVersion())
	}
	if metadata.GetHashAlgo() != sketchpb.HashAlgorithm_HASH_ALGORITHM_HMAC_SHA256_64 {
		return nil, fmt.Errorf("%w: hash algorithm %s", ErrInvalidWireEncoding, metadata.GetHashAlgo())
	}
	if metadata.GetRepresentationMode() != sketchpb.RepresentationMode_REPRESENTATION_MODE_FREQUENT_ITEMS_BOUNDED_MAP {
		return nil, fmt.Errorf("%w: representation %s", ErrInvalidWireEncoding, metadata.GetRepresentationMode())
	}

	profile := Profile(metadata.GetProfile())
	mapSize := int(metadata.GetFrequentItemsMapSize())
	sketch, err := newSketch(
		profile,
		mapSize,
		sketchhash.Domain(metadata.GetHashDomain()),
		sketchhash.HMACSHA25664,
	)
	if err != nil {
		return nil, err
	}
	if !sketchhash.IsRegisteredDomain(sketch.domain) {
		return nil, fmt.Errorf("%w: %s", sketchhash.ErrUnregisteredDomain, sketch.domain)
	}
	if body.GetTotalWeight() < 0 || body.GetMaxError() < 0 || body.GetMaxError() > body.GetTotalWeight() {
		return nil, fmt.Errorf("%w: invalid total/error", ErrInvalidWireEncoding)
	}
	if len(body.GetEntries()) > sketch.mapSize {
		return nil, fmt.Errorf("%w: too many entries", ErrInvalidWireEncoding)
	}

	sketch.totalWeight = body.GetTotalWeight()
	sketch.maxError = body.GetMaxError()
	var previous uint64
	for i, entry := range body.GetEntries() {
		hash := entry.GetHash()
		estimate := entry.GetEstimate()
		errorValue := entry.GetError()
		if i > 0 && hash <= previous {
			return nil, fmt.Errorf("%w: entries unsorted", ErrInvalidWireEncoding)
		}
		if estimate <= 0 || errorValue != sketch.maxError || estimate <= sketch.maxError {
			return nil, fmt.Errorf("%w: invalid entry estimate/error", ErrInvalidWireEncoding)
		}
		sketch.insertCounter(hash, estimate)
		previous = hash
	}

	return sketch, nil
}

func (s *Sketch) addResidual(hash uint64, residual int64) error {
	if residual <= 0 {
		return nil
	}
	current, ok := s.items[hash]
	if ok {
		if current.estimate > math.MaxInt64-residual {
			return fmt.Errorf("%w: counter", ErrWeightOverflow)
		}
		current.estimate += residual
		s.heapDown(current.heapIndex)
		return nil
	}
	if len(s.items) < s.mapSize {
		if s.maxError > math.MaxInt64-residual {
			return fmt.Errorf("%w: counter", ErrWeightOverflow)
		}
		s.insertCounter(hash, s.maxError+residual)
		return nil
	}

	consumed, err := s.pruneForResidual(residual)
	if err != nil {
		return err
	}
	remaining := residual - consumed
	if remaining > 0 {
		if len(s.items) >= s.mapSize {
			return fmt.Errorf("%w: no free counter", ErrInvalidMapSize)
		}
		if s.maxError > math.MaxInt64-remaining {
			return fmt.Errorf("%w: counter", ErrWeightOverflow)
		}
		s.insertCounter(hash, s.maxError+remaining)
	}

	return nil
}

func (s *Sketch) pruneForResidual(residual int64) (int64, error) {
	if len(s.heap) == 0 {
		return 0, nil
	}
	minResidual := s.heap[0].estimate - s.maxError
	if residual < minResidual {
		if s.maxError > math.MaxInt64-residual {
			return 0, fmt.Errorf("%w: max error", ErrWeightOverflow)
		}
		s.maxError += residual
		return residual, nil
	}

	if s.maxError > math.MaxInt64-minResidual {
		return 0, fmt.Errorf("%w: max error", ErrWeightOverflow)
	}
	s.maxError += minResidual
	s.removeExpired()
	return minResidual, nil
}

func (s *Sketch) removeExpired() {
	for len(s.heap) != 0 && s.heap[0].estimate <= s.maxError {
		s.removeAt(0)
	}
}

func (s *Sketch) insertCounter(hash uint64, estimate int64) {
	slot := s.free[len(s.free)-1]
	s.free = s.free[:len(s.free)-1]
	*slot = counter{
		hash:      hash,
		estimate:  estimate,
		heapIndex: len(s.heap),
	}
	s.items[hash] = slot
	s.heap = append(s.heap, slot)
	s.heapUp(slot.heapIndex)
}

func (s *Sketch) removeAt(index int) {
	removed := s.heap[index]
	last := s.heap[len(s.heap)-1]
	s.heap = s.heap[:len(s.heap)-1]
	if index < len(s.heap) {
		s.heap[index] = last
		last.heapIndex = index
		s.heapDown(index)
		s.heapUp(index)
	}
	delete(s.items, removed.hash)
	*removed = counter{}
	s.free = append(s.free, removed)
}

func (s *Sketch) heapUp(index int) {
	for index > 0 {
		parent := (index - 1) / 2
		if !lessCounter(s.heap[index], s.heap[parent]) {
			return
		}
		s.swapHeap(index, parent)
		index = parent
	}
}

func (s *Sketch) heapDown(index int) {
	for {
		left := 2*index + 1
		if left >= len(s.heap) {
			return
		}
		smallest := left
		right := left + 1
		if right < len(s.heap) && lessCounter(s.heap[right], s.heap[left]) {
			smallest = right
		}
		if !lessCounter(s.heap[smallest], s.heap[index]) {
			return
		}
		s.swapHeap(index, smallest)
		index = smallest
	}
}

func (s *Sketch) swapHeap(i int, j int) {
	s.heap[i], s.heap[j] = s.heap[j], s.heap[i]
	s.heap[i].heapIndex = i
	s.heap[j].heapIndex = j
}

func (s *Sketch) reset() {
	clear(s.items)
	s.heap = s.heap[:0]
	s.free = s.free[:0]
	for i := range s.pool {
		s.pool[i] = counter{}
		s.free = append(s.free, &s.pool[i])
	}
	s.totalWeight = 0
	s.maxError = 0
}

func (s *Sketch) itemsSortedByHash() []Item {
	items := make([]Item, 0, len(s.items))
	for _, current := range s.items {
		lower := maxInt64(0, current.estimate-s.maxError)
		items = append(items, Item{
			Hash:       current.hash,
			Estimate:   current.estimate,
			LowerBound: lower,
			UpperBound: current.estimate,
			Error:      current.estimate - lower,
		})
	}
	sort.Slice(items, func(i int, j int) bool {
		return items[i].Hash < items[j].Hash
	})

	return items
}

func addCombinedResidual(combined map[uint64]int64, hash uint64, residual int64) {
	if residual <= 0 {
		return
	}
	if combined[hash] > math.MaxInt64-residual {
		combined[hash] = math.MaxInt64
		return
	}
	combined[hash] += residual
}

func lessCounter(left *counter, right *counter) bool {
	if left.estimate != right.estimate {
		return left.estimate < right.estimate
	}

	return left.hash < right.hash
}

func maxInt64(left int64, right int64) int64 {
	if left > right {
		return left
	}

	return right
}
