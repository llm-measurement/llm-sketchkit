// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

// Package sketchcheck shares ordered metadata checks without owning public errors.
package sketchcheck

import (
	"fmt"

	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
	sketchpb "github.com/llm-measurement/llm-sketchkit/go/sketchkit/internal/pb"
)

// Header checks kind, version, and algorithm in their wire-error precedence order.
func Header(metadata *sketchpb.SketchMetadata, kind sketchpb.SketchKind, version uint32, invalidWire error) error {
	if metadata.GetKind() != kind {
		return fmt.Errorf("%w: kind %s", invalidWire, metadata.GetKind())
	}
	if metadata.GetWireVersion() != version {
		return fmt.Errorf("%w: wire version %d", invalidWire, metadata.GetWireVersion())
	}
	if metadata.GetHashAlgo() != sketchpb.HashAlgorithm_HASH_ALGORITHM_HMAC_SHA256_64 {
		return fmt.Errorf("%w: hash algorithm %s", invalidWire, metadata.GetHashAlgo())
	}
	return nil
}

// Keying checks domain before algorithm and never includes caller-supplied strings.
func Keying(domain sketchhash.Domain, algorithm sketchhash.Algorithm, incompatibleMerge error) error {
	if !sketchhash.IsRegisteredDomain(domain) {
		return sketchhash.ErrUnregisteredDomain
	}
	if algorithm != sketchhash.HMACSHA25664 {
		return fmt.Errorf("%w: unsupported hash algorithm", incompatibleMerge)
	}
	return nil
}
