// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

package summary

import (
	"bytes"
	"encoding/json"
	"errors"
)

// checkJSONCounts walks the envelope's fixed object shape before Decode builds
// maps. Token decoding keeps oversized objects from allocating all their entries.
func checkJSONCounts(data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var object func(string, int) error
	object = func(kind string, limit int) error {
		token, err := d.Token()
		if err != nil || token != json.Delim('{') {
			return errors.New("invalid summary JSON")
		}
		for count := 0; d.More(); count++ {
			if count >= limit {
				return errors.New("invalid summary payload count")
			}
			key, err := d.Token()
			if err != nil {
				return errors.New("invalid summary JSON")
			}
			if kind == "root" && (key == "counters" || key == "sketches") {
				n := 128
				if key == "sketches" {
					n = 16
				}
				if err := object(key.(string), n); err != nil {
					return err
				}
			} else if kind == "sketches" {
				if err := object("payload", 2); err != nil {
					return err
				}
			} else {
				value, err := d.Token()
				if err != nil {
					return errors.New("invalid summary JSON")
				}
				if _, nested := value.(json.Delim); nested {
					return errors.New("invalid summary JSON")
				}
			}
		}
		_, err = d.Token()
		if err != nil {
			return errors.New("invalid summary JSON")
		}
		return nil
	}
	return object("root", 14)
}
