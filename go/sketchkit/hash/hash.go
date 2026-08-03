// Code authors: Vijay Erramilli and Codex
// Package hash implements domain-separated keyed hashing for sketch inputs.
package hash

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
)

// Algorithm names a hash algorithm from spec/hash.md.
type Algorithm string

const (
	// HMACSHA25664 is HMAC-SHA256 truncated to the first 64 bits.
	HMACSHA25664 Algorithm = "hmac_sha256_64"

	minSecretBytes = 16
)

// Domain names a registered hash domain from spec/hash.md.
type Domain string

const (
	PromptV1       Domain = "prompt:v1"
	UserV1         Domain = "user:v1"
	ToolV1         Domain = "tool:v1"
	RetrievalDocV1 Domain = "retrieval-doc:v1"
	SessionV1      Domain = "session:v1"
	MCPSessionV1   Domain = "mcp-session:v1"
	MCPMethodV1    Domain = "mcp-method:v1"
	ToolErrorV1    Domain = "tool-error:v1"
)

var registeredDomains = map[Domain]struct{}{
	PromptV1:       {},
	UserV1:         {},
	ToolV1:         {},
	RetrievalDocV1: {},
	SessionV1:      {},
	MCPSessionV1:   {},
	MCPMethodV1:    {},
	ToolErrorV1:    {},
}

var domainList = []Domain{
	PromptV1,
	UserV1,
	ToolV1,
	RetrievalDocV1,
	SessionV1,
	MCPSessionV1,
	MCPMethodV1,
	ToolErrorV1,
}

var (
	// ErrEmptySecret reports that no hash secret was supplied.
	ErrEmptySecret = errors.New("empty hash secret")

	// ErrWeakSecret reports secret material that is too short or is a known placeholder.
	ErrWeakSecret = errors.New("weak hash secret")

	// ErrEmptySecretEnv reports that no environment variable name was supplied.
	ErrEmptySecretEnv = errors.New("empty hash secret environment variable")

	// ErrUnregisteredDomain reports a domain that is absent from the registry.
	ErrUnregisteredDomain = errors.New("unregistered hash domain")
)

// Secret is opaque keyed-hash secret material.
type Secret struct {
	value []byte
}

// SecretFromEnv loads opaque secret bytes from envName.
func SecretFromEnv(envName string) (Secret, error) {
	if envName == "" {
		return Secret{}, ErrEmptySecretEnv
	}

	value := os.Getenv(envName)
	if value == "" {
		return Secret{}, fmt.Errorf("%w: %s", ErrEmptySecret, envName)
	}
	if err := validateSecretBytes([]byte(value)); err != nil {
		return Secret{}, err
	}

	return Secret{value: []byte(value)}, nil
}

// String redacts secret material in ordinary formatting.
func (Secret) String() string {
	return "<redacted hash secret>"
}

// GoString redacts secret material in %#v formatting.
func (Secret) GoString() string {
	return "hash.Secret(<redacted>)"
}

// Domains returns the registered v0.1 hash domains.
func Domains() []Domain {
	domains := make([]Domain, len(domainList))
	copy(domains, domainList)

	return domains
}

// IsRegisteredDomain reports whether domain is registered in spec/hash.md.
func IsRegisteredDomain(domain Domain) bool {
	_, ok := registeredDomains[domain]
	return ok
}

// Hash64 returns HMAC-SHA256(secret, domain || 0x00 || canonicalBytes) truncated to uint64.
func Hash64(secret Secret, domain Domain, canonicalBytes []byte) (uint64, error) {
	digest, err := Digest64(secret, domain, canonicalBytes)
	if err != nil {
		return 0, err
	}

	return binary.BigEndian.Uint64(digest[:]), nil
}

// Digest64 returns the first eight HMAC-SHA256 digest bytes.
func Digest64(secret Secret, domain Domain, canonicalBytes []byte) ([8]byte, error) {
	if err := validateSecretBytes(secret.value); err != nil {
		return [8]byte{}, err
	}
	if !IsRegisteredDomain(domain) {
		return [8]byte{}, fmt.Errorf("%w: %s", ErrUnregisteredDomain, domain)
	}

	mac := hmac.New(sha256.New, secret.value)
	mac.Write([]byte(domain))
	mac.Write([]byte{0x00})
	mac.Write(canonicalBytes)

	sum := mac.Sum(nil)
	var digest [8]byte
	copy(digest[:], sum[:8])

	return digest, nil
}

func validateSecretBytes(value []byte) error {
	if len(value) == 0 {
		return ErrEmptySecret
	}
	if len(value) < minSecretBytes {
		return fmt.Errorf("%w: must be at least %d bytes", ErrWeakSecret, minSecretBytes)
	}
	switch string(value) {
	case "demo-secret-change-me", "dev-secret-change-me":
		return fmt.Errorf("%w: placeholder must be replaced", ErrWeakSecret)
	}
	return nil
}

// Digest64Hex returns the lowercase fixed-width hexadecimal digest.
func Digest64Hex(secret Secret, domain Domain, canonicalBytes []byte) (string, error) {
	digest, err := Digest64(secret, domain, canonicalBytes)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(digest[:]), nil
}
