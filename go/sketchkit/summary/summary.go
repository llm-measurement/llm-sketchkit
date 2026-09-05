// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay Erramilli and Codex

// Package summary exchanges window-scoped counters and existing sketch state.
// Producer trust, disjoint input ownership, and secret distribution are external.
package summary

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"slices"
	"sort"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/bloom"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/frequentitems"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/hllpp"
	sketchpb "github.com/llm-measurement/llm-sketchkit/go/sketchkit/internal/pb"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/minhash"
	"google.golang.org/protobuf/proto"
)

const MaxBytes = 8 << 20
const maxBatchBytes = 64 << 20
const maxDuration = int64(24 * 60 * 60 * 1e9)

var identifier = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

// Payload holds canonical protobuf bytes, base64-encoded by JSON.
type Payload struct {
	Data []byte `json:"data"`
	Kind string `json:"kind"`
}

// Envelope is a cumulative snapshot of one producer epoch within one window.
// Field order is lexical to match the canonical JSON contract in both languages.
type Envelope struct {
	AccountingID   string             `json:"accounting_id"`
	Counters       map[string]uint64  `json:"counters"`
	EmittedAt      int64              `json:"emitted_at_unix_nano"`
	Epoch          string             `json:"epoch"`
	KeyID          string             `json:"key_id"`
	ObservedEnd    int64              `json:"observed_end_unix_nano"`
	ObservedStart  int64              `json:"observed_start_unix_nano"`
	ProducerID     string             `json:"producer_id"`
	ScopeID        string             `json:"scope_id"`
	Sequence       uint64             `json:"sequence"`
	Sketches       map[string]Payload `json:"sketches"`
	Version        int                `json:"version"`
	WindowDuration int64              `json:"window_duration_unix_nano"`
	WindowStart    int64              `json:"window_start_unix_nano"`
}

func (e Envelope) Validate() error {
	if e.Version != 1 || e.Sequence == 0 || e.Sequence > math.MaxInt64 {
		return errors.New("invalid summary version or sequence")
	}
	for _, id := range []string{e.AccountingID, e.Epoch, e.KeyID, e.ProducerID, e.ScopeID} {
		if !identifier.MatchString(id) {
			return errors.New("invalid summary identifier")
		}
	}
	if e.WindowDuration <= 0 || e.WindowDuration > maxDuration || e.WindowStart < 0 ||
		e.WindowStart > math.MaxInt64-e.WindowDuration || e.WindowStart%e.WindowDuration != 0 ||
		e.ObservedStart < e.WindowStart || e.ObservedEnd < e.ObservedStart ||
		e.ObservedEnd > e.WindowStart+e.WindowDuration || e.EmittedAt < e.ObservedEnd {
		return errors.New("invalid summary observation interval")
	}
	if e.Counters == nil || len(e.Counters) > 128 || e.Sketches == nil || len(e.Sketches) > 16 {
		return errors.New("invalid summary payload count")
	}
	for name, count := range e.Counters {
		if !identifier.MatchString(name) || count > math.MaxInt64 {
			return errors.New("invalid summary counter")
		}
	}
	size := 0
	for name, payload := range e.Sketches {
		size += len(payload.Data)
		if !identifier.MatchString(name) || size > MaxBytes {
			return errors.New("invalid summary sketch")
		}
		canonical, err := mergePayload(payload, nil)
		if err != nil || !bytes.Equal(canonical.Data, payload.Data) {
			return errors.New("invalid summary sketch state")
		}
	}
	return nil
}

// MarshalBinary returns canonical JSON. Errors leave the receiver unchanged.
func (e Envelope) MarshalBinary() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxBytes {
		return nil, errors.New("summary exceeds size limit")
	}
	return data, nil
}

// Parse rejects unknown fields, duplicates, noncanonical JSON, and invalid state.
func Parse(data []byte) (Envelope, error) {
	var e Envelope
	if len(data) > MaxBytes {
		return e, errors.New("summary exceeds size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&e); err != nil {
		return Envelope{}, errors.New("invalid summary JSON")
	}
	canonical, err := e.MarshalBinary()
	if err != nil {
		return Envelope{}, err
	}
	if !bytes.Equal(canonical, data) {
		return Envelope{}, errors.New("noncanonical summary JSON")
	}
	return e, nil
}

// Compatible checks measurement and sketch compatibility for comparison.
// Window starts may differ; durations must match. It does not mutate either input.
func Compatible(a, b Envelope) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if err := b.Validate(); err != nil {
		return err
	}
	if a.ScopeID != b.ScopeID || a.AccountingID != b.AccountingID || a.KeyID != b.KeyID ||
		a.WindowDuration != b.WindowDuration || !slices.Equal(names(a.Counters), names(b.Counters)) ||
		!slices.Equal(names(a.Sketches), names(b.Sketches)) {
		return errors.New("incompatible summary measurement contract")
	}
	for name, payload := range a.Sketches {
		other := b.Sketches[name]
		var left, right sketchpb.Sketch
		if err := proto.Unmarshal(payload.Data, &left); err != nil {
			return err
		}
		if err := proto.Unmarshal(other.Data, &right); err != nil {
			return err
		}
		left.Metadata.RepresentationMode = 0
		right.Metadata.RepresentationMode = 0
		if payload.Kind != other.Kind || !proto.Equal(left.Metadata, right.Metadata) {
			return errors.New("incompatible summary sketch metadata")
		}
	}
	return nil
}

type Source struct {
	ProducerID    string `json:"producer_id"`
	Epoch         string `json:"epoch"`
	Sequence      uint64 `json:"sequence"`
	ObservedStart int64  `json:"observed_start_unix_nano"`
	ObservedEnd   int64  `json:"observed_end_unix_nano"`
}

// Result is new combined state. Missing and Partial refer to expected producers.
type Result struct {
	Counters map[string]uint64  `json:"counters"`
	Sketches map[string]Payload `json:"sketches"`
	Sources  []Source           `json:"sources"`
	Missing  []string           `json:"missing"`
	Partial  []string           `json:"partial"`
}

// Combine selects the latest cumulative snapshot per producer/epoch and rebuilds
// one window. expected declares disjoint input owners, not merely allowed names.
// Errors return no partial result and never modify the supplied envelopes.
func Combine(input []Envelope, expected []string) (Result, error) {
	if len(input) > 1024 || len(expected) == 0 || len(expected) > 128 {
		return Result{}, errors.New("invalid summary batch size")
	}
	owners := make(map[string]bool, len(expected))
	for _, id := range expected {
		if !identifier.MatchString(id) {
			return Result{}, errors.New("invalid expected producer")
		}
		if _, ok := owners[id]; ok {
			return Result{}, errors.New("duplicate expected producer")
		}
		owners[id] = false
	}
	// Sorting a copy makes replacement and merge order independent of arrival order.
	docs := append([]Envelope(nil), input...)
	sort.Slice(docs, func(i, j int) bool {
		a, b := docs[i], docs[j]
		if a.ProducerID != b.ProducerID {
			return a.ProducerID < b.ProducerID
		}
		if a.Epoch != b.Epoch {
			return a.Epoch < b.Epoch
		}
		return a.Sequence < b.Sequence
	})
	selected := make([]Envelope, 0, len(docs))
	encodedBytes := 0
	var previousBytes []byte
	for _, doc := range docs {
		encoded, err := doc.MarshalBinary()
		if err != nil {
			return Result{}, err
		}
		encodedBytes += len(encoded)
		if encodedBytes > maxBatchBytes {
			return Result{}, errors.New("summary batch exceeds size limit")
		}
		if _, ok := owners[doc.ProducerID]; !ok {
			return Result{}, errors.New("unexpected summary producer")
		}
		if len(selected) > 0 {
			if doc.WindowStart != selected[0].WindowStart {
				return Result{}, errors.New("cannot combine different windows")
			}
			if err := Compatible(selected[0], doc); err != nil {
				return Result{}, err
			}
			last := selected[len(selected)-1]
			if last.ProducerID == doc.ProducerID && last.Epoch == doc.Epoch {
				if last.Sequence == doc.Sequence {
					if !bytes.Equal(previousBytes, encoded) {
						return Result{}, errors.New("conflicting summary sequence")
					}
					continue
				}
				if doc.ObservedStart != last.ObservedStart || doc.ObservedEnd < last.ObservedEnd {
					return Result{}, errors.New("summary observation regressed")
				}
				for name, value := range last.Counters {
					if doc.Counters[name] < value {
						return Result{}, errors.New("summary counter regressed")
					}
				}
				selected[len(selected)-1] = doc
				previousBytes = encoded
				continue
			}
		}
		selected = append(selected, doc)
		previousBytes = encoded
	}
	result := Result{Counters: map[string]uint64{}, Sketches: map[string]Payload{}, Sources: []Source{}, Missing: []string{}, Partial: []string{}}
	intervals := make(map[string][]Envelope)
	for _, doc := range selected {
		owners[doc.ProducerID] = true
		intervals[doc.ProducerID] = append(intervals[doc.ProducerID], doc)
		result.Sources = append(result.Sources, Source{doc.ProducerID, doc.Epoch, doc.Sequence, doc.ObservedStart, doc.ObservedEnd})
		for name, value := range doc.Counters {
			if result.Counters[name] > math.MaxInt64-value {
				return Result{}, errors.New("combined counter overflow")
			}
			result.Counters[name] += value
		}
		for name, payload := range doc.Sketches {
			var merged Payload
			var err error
			if previous, ok := result.Sketches[name]; ok {
				merged, err = mergePayload(previous, &payload)
			} else {
				merged, err = mergePayload(payload, nil)
			}
			if err != nil {
				return Result{}, err
			}
			result.Sketches[name] = merged
		}
	}
	for _, id := range names(owners) {
		if !owners[id] {
			result.Missing = append(result.Missing, id)
			continue
		}
		parts := intervals[id]
		sort.Slice(parts, func(i, j int) bool { return parts[i].ObservedStart < parts[j].ObservedStart })
		end := parts[0].WindowStart
		partial := false
		for _, part := range parts {
			if part.ObservedStart < end {
				return Result{}, errors.New("overlapping producer epochs")
			}
			partial = partial || part.ObservedStart != end
			end = part.ObservedEnd
		}
		if partial || end != parts[0].WindowStart+parts[0].WindowDuration {
			result.Partial = append(result.Partial, id)
		}
	}
	return result, nil
}

func names[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func mergePayload(a Payload, b *Payload) (Payload, error) {
	if b != nil && a.Kind != b.Kind {
		return Payload{}, errors.New("sketch kinds differ")
	}
	var data []byte
	var err error
	switch a.Kind {
	case "hllpp":
		s, parseErr := hllpp.Parse(a.Data)
		if parseErr != nil {
			return Payload{}, parseErr
		}
		if b != nil {
			other, parseErr := hllpp.Parse(b.Data)
			if parseErr != nil {
				return Payload{}, parseErr
			}
			if err = s.Merge(other); err != nil {
				return Payload{}, err
			}
		}
		data, err = s.MarshalBinary()
	case "frequent_items":
		s, parseErr := frequentitems.Parse(a.Data)
		if parseErr != nil {
			return Payload{}, parseErr
		}
		if b != nil {
			other, parseErr := frequentitems.Parse(b.Data)
			if parseErr != nil {
				return Payload{}, parseErr
			}
			if err = s.Merge(other); err != nil {
				return Payload{}, err
			}
		}
		data, err = s.MarshalBinary()
	case "bloom":
		s, parseErr := bloom.Parse(a.Data)
		if parseErr != nil {
			return Payload{}, parseErr
		}
		if b != nil {
			other, parseErr := bloom.Parse(b.Data)
			if parseErr != nil {
				return Payload{}, parseErr
			}
			if err = s.Merge(other); err != nil {
				return Payload{}, err
			}
		}
		data, err = s.MarshalBinary()
	case "minhash":
		s, parseErr := minhash.Parse(a.Data)
		if parseErr != nil {
			return Payload{}, parseErr
		}
		if b != nil {
			other, parseErr := minhash.Parse(b.Data)
			if parseErr != nil {
				return Payload{}, parseErr
			}
			if err = s.Merge(other); err != nil {
				return Payload{}, err
			}
		}
		data, err = s.MarshalBinary()
	default:
		return Payload{}, errors.New("unknown summary sketch kind")
	}
	return Payload{Data: data, Kind: a.Kind}, err
}
