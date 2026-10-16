// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

package summary

import (
	"bytes"
	"runtime"
	"testing"
)

func TestCountsBeforeMapAllocation(t *testing.T) {
	for _, key := range []string{"counters", "sketches"} {
		data := append([]byte(`{"`+key+`":{`), bytes.Repeat([]byte(`"a":0,`), 500000)...)
		data = append(data, []byte(`"z":0}}`)...)
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		_, err := Parse(data)
		runtime.ReadMemStats(&after)
		if err == nil {
			t.Fatal("accepted oversized map")
		}
		if n := after.TotalAlloc - before.TotalAlloc; n > 1<<20 {
			t.Fatalf("allocated %d bytes", n)
		}
	}
}
