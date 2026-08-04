// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay Erramilli and Codex
// Package canon implements llm-sketchkit canonicalization profiles.
package canon

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Profile names a canonicalization profile from spec/canonicalization.md.
type Profile string

const (
	// TextV1 applies NFC normalization, newline normalization, and Unicode trim.
	TextV1 Profile = "text_v1"
)

var (
	// ErrUnsupportedProfile reports a profile that is not implemented.
	ErrUnsupportedProfile = errors.New("unsupported canonicalization profile")

	// ErrInvalidUTF8 reports input that cannot be decoded as UTF-8.
	ErrInvalidUTF8 = errors.New("invalid utf-8 input")
)

// Canonicalize applies profile to UTF-8 bytes and returns canonical UTF-8 bytes.
func Canonicalize(profile Profile, input []byte) ([]byte, error) {
	if profile != TextV1 {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedProfile, profile)
	}
	if !utf8.Valid(input) {
		return nil, ErrInvalidUTF8
	}

	return canonicalizeTextV1(string(input)), nil
}

// CanonicalizeString applies profile to a Go string and returns canonical UTF-8 bytes.
func CanonicalizeString(profile Profile, input string) ([]byte, error) {
	if profile != TextV1 {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedProfile, profile)
	}
	if !utf8.ValidString(input) {
		return nil, ErrInvalidUTF8
	}

	return canonicalizeTextV1(input), nil
}

func canonicalizeTextV1(input string) []byte {
	normalized := norm.NFC.String(input)
	normalized = strings.ReplaceAll(normalized, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = strings.TrimFunc(normalized, unicode.IsSpace)
	normalized = norm.NFC.String(normalized)

	return []byte(normalized)
}
