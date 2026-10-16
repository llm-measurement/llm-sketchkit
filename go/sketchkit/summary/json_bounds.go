// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

package summary

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/modelusage"
)

var shapes = map[string][]string{
	"root":       {"accounting_id", "counters", "emitted_at_unix_nano", "epoch", "key_id", "model_usage", "observed_end_unix_nano", "observed_start_unix_nano", "producer_id", "scope_id", "sequence", "sketches", "version", "window_duration_unix_nano", "window_start_unix_nano"},
	"extension":  {"contract_id", "histogram_id", "models", "version"},
	"row":        {"cache_partition_violations", "complete", "excluded", "fields", "histogram", "provenance", "requests"},
	"fields":     {"input", "output", "cache_read", "cache_write"},
	"field":      {"tokens", "reported", "missing", "invalid", "conflict", "subset_violation"},
	"cohort":     {"requests", "input_tokens", "output_tokens", "cache_read_input_tokens", "cache_write_input_tokens", "input_min", "input_max"},
	"bin":        {"reported", "complete"},
	"reported":   {"requests", "input_tokens"},
	"excluded":   {"invalid_usage", "missing_split", "zero_origin_unknown"},
	"provenance": {"input", "output"},
	"origin":     {"provider_reported", "inferred", "unavailable", "unknown"},
	"payload":    {"data", "kind"},
}

func checkJSONCounts(data []byte) error { _, err := scanShape(data); return err }

// HasModelUsage performs bounded structural capability detection. Malformed
// data retains a validation error; substrings in values are not declarations.
func HasModelUsage(data []byte) (bool, error) {
	if len(data) > MaxBytes {
		return false, &modelusage.Error{Reason: "size"}
	}
	return scanShape(data)
}

// scanShape bounds containers before per-row semantic checks and map decoding.
// Missing-member checks are deferred so an excess real-model count wins before
// validating deliberately empty rows. Maximum container depth is seven.
func scanShape(data []byte) (bool, error) {
	if !validJSONUnicode(data) {
		return false, &modelusage.Error{Reason: "structure"}
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	bad := func() error { return &modelusage.Error{Reason: "structure"} }
	present, shapeError := false, false
	var object func(string) error
	object = func(kind string) error {
		limit := len(shapes[kind])
		switch kind {
		case "counters":
			limit = 128
		case "sketches":
			limit = 16
		case "models":
			limit = 5004
		}
		token, err := d.Token()
		if err != nil || token != json.Delim('{') {
			return bad()
		}
		seen := map[string]bool{}
		real := 0
		for count := 0; d.More(); count++ {
			if count >= limit {
				if kind == "models" {
					return &modelusage.Error{Reason: "model_count"}
				}
				return bad()
			}
			keyToken, err := d.Token()
			key, ok := keyToken.(string)
			if err != nil || !ok || seen[key] {
				return bad()
			}
			seen[key] = true
			child := ""
			switch kind {
			case "root":
				switch key {
				case "counters", "sketches":
					child = key
				case "model_usage":
					child = "extension"
					present = true
				}
			case "extension":
				if key == "models" {
					child = "models"
				}
			case "models":
				if !modelusage.IsReserved(key) {
					real++
					if real > modelusage.MaxModels {
						return &modelusage.Error{Reason: "model_count"}
					}
					if !modelusage.IsPublicID(key) {
						return bad()
					}
				}
				child = "row"
			case "sketches":
				child = "payload"
			case "fields":
				child = "field"
			case "provenance":
				child = "origin"
			case "row":
				switch key {
				case "complete":
					child = "cohort"
				case "fields", "histogram", "excluded", "provenance":
					child = key
				}
			case "bin":
				switch key {
				case "reported":
					child = "reported"
				case "complete":
					child = "cohort"
				}
			}
			if child == "histogram" {
				token, err := d.Token()
				if err != nil || token != json.Delim('[') {
					return bad()
				}
				for i := 0; i < 4; i++ {
					if !d.More() {
						return bad()
					}
					if err := object("bin"); err != nil {
						return err
					}
				}
				if d.More() {
					return bad()
				}
				token, err = d.Token()
				if err != nil || token != json.Delim(']') {
					return bad()
				}
			} else if child != "" {
				if err := object(child); err != nil {
					return err
				}
			} else {
				value, err := d.Token()
				if err != nil {
					return bad()
				}
				if _, nested := value.(json.Delim); nested {
					return bad()
				}
			}
		}
		if required, ok := shapes[kind]; ok {
			n := len(required)
			if kind == "root" && !seen["model_usage"] {
				n--
			}
			if len(seen) != n {
				shapeError = true
			}
			for _, k := range required {
				if k == "model_usage" && kind == "root" {
					continue
				}
				if !seen[k] {
					shapeError = true
				}
			}
		}
		token, err = d.Token()
		if err != nil || token != json.Delim('}') {
			return bad()
		}
		return nil
	}
	if err := object("root"); err != nil {
		return false, err
	}
	if shapeError || int(d.InputOffset()) != len(data) {
		return false, bad()
	}
	if _, err := d.Token(); err != io.EOF {
		return false, bad()
	}
	return present, nil
}

// encoding/json repairs invalid Unicode. Check the original bytes and escape
// pairs before token decoding so capability detection never accepts that repair.
func validJSONUnicode(data []byte) bool {
	if !utf8.Valid(data) {
		return false
	}
	for i := 0; i < len(data); i++ {
		if data[i] != '\\' {
			continue
		}
		i++
		if i >= len(data) {
			return false
		}
		if data[i] != 'u' {
			continue
		}
		if i+4 >= len(data) {
			return false
		}
		hi, err := strconv.ParseUint(string(data[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if !utf16.IsSurrogate(rune(hi)) {
			continue
		}
		if hi >= 0xdc00 || i+6 >= len(data) || data[i+1] != '\\' || data[i+2] != 'u' {
			return false
		}
		lo, err := strconv.ParseUint(string(data[i+3:i+7]), 16, 16)
		if err != nil || lo < 0xdc00 || lo > 0xdfff {
			return false
		}
		i += 6
	}
	return true
}
