// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

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
	return e.validate(nil)
}

func (e Envelope) validate(parsed map[string]*parsedPayload) error {
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
		state, canonical, err := parsePayload(payload)
		if err != nil || !bytes.Equal(canonical, payload.Data) {
			return errors.New("invalid summary sketch state")
		}
		if parsed != nil {
			parsed[name] = state
		}
	}
	return nil
}

// MarshalBinary returns canonical JSON. Errors leave the receiver unchanged.
func (e Envelope) MarshalBinary() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	return e.marshalValidated()
}

func (e Envelope) marshalValidated() ([]byte, error) {
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
	if err := checkJSONCounts(data); err != nil {
		return e, err
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
	left, right := map[string]*parsedPayload{}, map[string]*parsedPayload{}
	if err := a.validate(left); err != nil {
		return err
	}
	if err := b.validate(right); err != nil {
		return err
	}
	return compatibleValidated(a, b, left, right)
}

func compatibleValidated(a, b Envelope, left, right map[string]*parsedPayload) error {
	if a.ScopeID != b.ScopeID || a.AccountingID != b.AccountingID || a.KeyID != b.KeyID ||
		a.WindowDuration != b.WindowDuration || !slices.Equal(names(a.Counters), names(b.Counters)) ||
		!slices.Equal(names(a.Sketches), names(b.Sketches)) {
		return errors.New("incompatible summary measurement contract")
	}
	for name, payload := range a.Sketches {
		other := b.Sketches[name]
		if payload.Kind != other.Kind || !proto.Equal(left[name].metadata, right[name].metadata) {
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
	var reference, pending map[string]*parsedPayload
	result := Result{Counters: map[string]uint64{}, Sketches: map[string]Payload{}, Sources: []Source{}, Missing: []string{}, Partial: []string{}}
	accumulated := map[string]*parsedPayload{}
	accumulate := func(doc Envelope, parsed map[string]*parsedPayload) error {
		for name, value := range doc.Counters {
			if result.Counters[name] > math.MaxInt64-value {
				return errors.New("combined counter overflow")
			}
			result.Counters[name] += value
		}
		for name, state := range parsed {
			if previous, ok := accumulated[name]; ok {
				if err := previous.merge(state); err != nil {
					return err
				}
			} else {
				accumulated[name] = state
			}
		}
		return nil
	}
	// Accumulate finalized groups, but input validation and selection errors must
	// still precede every counter or sketch merge error.
	var accumulationErr error
	encodedBytes := 0
	var previousBytes []byte
	for _, doc := range docs {
		parsed := map[string]*parsedPayload{}
		if err := doc.validate(parsed); err != nil {
			return Result{}, err
		}
		encoded, err := doc.marshalValidated()
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
			if err := compatibleValidated(selected[0], doc, reference, parsed); err != nil {
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
				pending = parsed
				previousBytes = encoded
				continue
			}
			if accumulationErr == nil {
				accumulationErr = accumulate(last, pending)
			}
		} else {
			reference = parsed
		}
		selected = append(selected, doc)
		pending = parsed
		previousBytes = encoded
	}
	if len(selected) > 0 && accumulationErr == nil {
		accumulationErr = accumulate(selected[len(selected)-1], pending)
	}
	if accumulationErr != nil {
		return Result{}, accumulationErr
	}
	intervals := make(map[string][]Envelope)
	for _, doc := range selected {
		owners[doc.ProducerID] = true
		intervals[doc.ProducerID] = append(intervals[doc.ProducerID], doc)
		result.Sources = append(result.Sources, Source{doc.ProducerID, doc.Epoch, doc.Sequence, doc.ObservedStart, doc.ObservedEnd})
	}
	for name, parsed := range accumulated {
		data, err := parsed.marshal()
		if err != nil {
			return Result{}, err
		}
		result.Sketches[name] = Payload{Kind: parsed.kind, Data: data}
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
