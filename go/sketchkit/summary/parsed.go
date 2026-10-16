// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

package summary

import (
	"errors"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/bloom"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/frequentitems"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/hllpp"
	sketchpb "github.com/llm-measurement/llm-sketchkit/go/sketchkit/internal/pb"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/minhash"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

// parsedPayload is owned by one operation, never by an input envelope or a
// global cache. Its metadata remains immutable while the sketch accumulates.
type parsedPayload struct {
	kind     string
	metadata *sketchpb.SketchMetadata
	h        *hllpp.Sketch
	f        *frequentitems.Sketch
	b        *bloom.Sketch
	m        *minhash.Sketch
}

func parsePayload(p Payload) (*parsedPayload, []byte, error) {
	s := &parsedPayload{kind: p.Kind}
	var err error
	switch p.Kind {
	case "hllpp":
		s.h, err = hllpp.Parse(p.Data)
	case "frequent_items":
		s.f, err = frequentitems.Parse(p.Data)
	case "bloom":
		s.b, err = bloom.Parse(p.Data)
	case "minhash":
		s.m, err = minhash.Parse(p.Data)
	default:
		return nil, nil, errors.New("unknown summary sketch kind")
	}
	if err != nil {
		return nil, nil, err
	}
	canonical, err := s.marshal()
	if err != nil {
		return nil, nil, err
	}
	// Canonical writers put metadata first. Read only that small header instead
	// of decoding the repeated sketch body again for contract comparison.
	_, _, n := protowire.ConsumeTag(canonical)
	if n < 0 {
		return nil, nil, errors.New("invalid summary sketch state")
	}
	header, n := protowire.ConsumeBytes(canonical[n:])
	if n < 0 {
		return nil, nil, errors.New("invalid summary sketch state")
	}
	s.metadata = &sketchpb.SketchMetadata{}
	if err := proto.Unmarshal(header, s.metadata); err != nil {
		return nil, nil, err
	}
	s.metadata.RepresentationMode = 0
	return s, canonical, nil
}

func (s *parsedPayload) marshal() ([]byte, error) {
	switch s.kind {
	case "hllpp":
		return s.h.MarshalBinary()
	case "frequent_items":
		return s.f.MarshalBinary()
	case "bloom":
		return s.b.MarshalBinary()
	case "minhash":
		return s.m.MarshalBinary()
	default:
		return nil, errors.New("unknown summary sketch kind")
	}
}

func (s *parsedPayload) merge(other *parsedPayload) error {
	switch s.kind {
	case "hllpp":
		return s.h.Merge(other.h)
	case "frequent_items":
		if other.f.TotalWeight() == 0 {
			return nil
		}
		return s.f.Merge(other.f)
	case "bloom":
		return s.b.Merge(other.b)
	case "minhash":
		return s.m.Merge(other.m)
	default:
		return errors.New("unknown summary sketch kind")
	}
}
