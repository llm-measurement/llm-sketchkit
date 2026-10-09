// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

package summary

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func FuzzParse(f *testing.F) {
	f.Add([]byte(`{}`))
	for _, name := range []string{"envelope.json", "combined.json"} {
		data, err := os.ReadFile("../../../vectors/summaries/" + name)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		e, err := Parse(data)
		if err != nil {
			return
		}
		before, err := e.MarshalBinary()
		if err != nil || !bytes.Equal(data, before) {
			t.Fatal("parse changed canonical bytes")
		}
		one, err := Combine([]Envelope{e}, []string{e.ProducerID})
		if err != nil {
			t.Fatal(err)
		}
		two, err := Combine([]Envelope{e, e}, []string{e.ProducerID})
		if err != nil {
			t.Fatal(err)
		}
		a, err := json.Marshal(one)
		if err != nil {
			t.Fatal(err)
		}
		b, err := json.Marshal(two)
		if err != nil || !bytes.Equal(a, b) {
			t.Fatal("duplicate changed result")
		}
		after, err := e.MarshalBinary()
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("combine mutated input")
		}
	})
}
