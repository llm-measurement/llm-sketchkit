// Code authors: Vijay Erramilli and Codex
package canon

import (
	"errors"
	"testing"
)

func TestCanonicalizeTextV1(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "trim and newline normalization",
			input: "  Hello, sketchkit!\r\n",
			want:  "Hello, sketchkit!",
		},
		{
			name:  "empty after trim",
			input: " \t\r\n ",
			want:  "",
		},
		{
			name:  "unicode nfc",
			input: "  Cafe\u0301\r\n",
			want:  "Caf\u00e9",
		},
		{
			name:  "lone carriage return",
			input: "a\rb",
			want:  "a\nb",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := CanonicalizeString(TextV1, tt.input)
			if err != nil {
				t.Fatalf("CanonicalizeString() error = %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("CanonicalizeString() = %q, want %q", string(got), tt.want)
			}
		})
	}
}

func TestCanonicalizeRejectsInvalidUTF8(t *testing.T) {
	t.Parallel()

	_, err := Canonicalize(TextV1, []byte{0xff})
	if !errors.Is(err, ErrInvalidUTF8) {
		t.Fatalf("Canonicalize() error = %v, want %v", err, ErrInvalidUTF8)
	}
}

func TestCanonicalizeRejectsUnsupportedProfile(t *testing.T) {
	t.Parallel()

	for _, profile := range []Profile{
		"text_v1_casefold",
		"text_v1_fold_ws",
		"text_v1_casefold_fold_ws",
	} {
		profile := profile
		t.Run(string(profile), func(t *testing.T) {
			t.Parallel()

			_, err := Canonicalize(profile, []byte("hello"))
			if !errors.Is(err, ErrUnsupportedProfile) {
				t.Fatalf("Canonicalize() error = %v, want %v", err, ErrUnsupportedProfile)
			}
		})
	}
}
