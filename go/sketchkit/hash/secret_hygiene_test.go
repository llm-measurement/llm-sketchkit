// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

package hash_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
)

func TestSecretFormattingAllVerbs(t *testing.T) {
	t.Setenv("WP3_SECRET", "0123456789abcdef-PRIVATE_WP3_SENTINEL")
	secret, err := sketchhash.SecretFromEnv("WP3_SECRET")
	if err != nil {
		t.Fatal(err)
	}
	type exported struct{ Secret sketchhash.Secret }
	type unexported struct{ secret sketchhash.Secret }
	values := []any{secret, &secret, exported{secret}, unexported{secret}, &exported{secret}, &unexported{secret}}
	for _, verb := range []string{"%v", "%+v", "%#v", "%d", "%c", "%x", "%s", "%q", "%b", "%U", "%t", "%p"} {
		t.Run(verb, func(t *testing.T) {
			for _, value := range values {
				formatted := fmt.Sprintf(verb, value)
				for _, marker := range []string{"PRIVATE_WP3_SENTINEL", "0123456789", "48 49 50 51", "0x30, 0x31", "0 1 2 3", "30313233", "30 31 32 33", "110000 110001", "U+0030 U+0031"} {
					if strings.Contains(formatted, marker) {
						t.Fatalf("%s exposed secret material", verb)
					}
				}
			}
		})
	}
	data, err := json.Marshal(exported{secret})
	if err != nil || string(data) != `{"Secret":{}}` {
		t.Fatal("JSON serialization exposed secret state")
	}
}

func TestSecretRawEnvironmentBytes(t *testing.T) {
	t.Setenv("WP3_SECRET", "0123456789abcdef-\xff\xfe-WP3")
	secret, err := sketchhash.SecretFromEnv("WP3_SECRET")
	if err != nil {
		t.Fatal(err)
	}
	// This independent HMAC result is also asserted by the Python test.
	for range 2 {
		got, err := sketchhash.Digest64Hex(secret, sketchhash.PromptV1, []byte("hello"))
		if err != nil || got != "a981aba71dfda38f" {
			t.Fatalf("raw environment digest = %q, error = %v", got, err)
		}
	}
	_, err = sketchhash.Digest64(sketchhash.Secret{}, sketchhash.PromptV1, nil)
	if !errors.Is(err, sketchhash.ErrEmptySecret) {
		t.Fatalf("zero secret error = %v", err)
	}
}

func TestHashErrorMetadataHygiene(t *testing.T) {
	sentinel := "PRIVATE_WP3_SENTINEL\n\x1b[31m"
	t.Setenv(sentinel, "")
	_, err := sketchhash.SecretFromEnv(sentinel)
	if !errors.Is(err, sketchhash.ErrEmptySecret) || err.Error() != "empty hash secret" {
		t.Fatal("missing secret error exposed environment metadata")
	}
	t.Setenv("WP3_SECRET", "0123456789abcdef-test")
	secret, err := sketchhash.SecretFromEnv("WP3_SECRET")
	if err != nil {
		t.Fatal(err)
	}
	_, err = sketchhash.Digest64(secret, sketchhash.Domain(sentinel), nil)
	if !errors.Is(err, sketchhash.ErrUnregisteredDomain) || err.Error() != "unregistered hash domain" {
		t.Fatal("hash error exposed domain metadata")
	}
}
