// SPDX-License-Identifier: Apache-2.0

package modelusage

// Observation is an already alias-resolved component from base accounting.
// Nil Value is missing, not zero. Quality flags may overlap reported usage.
type Observation struct {
	Value           *uint64
	Invalid         bool
	Conflict        bool
	SubsetViolation bool
}
type Attempt struct {
	Input, Output, CacheRead, CacheWrite Observation
	InputOrigin, OutputOrigin            string
}

func flag(v bool) uint64 {
	if v {
		return 1
	}
	return 0
}
func origin(s string) (Origins, error) {
	switch s {
	case "", "unknown":
		return Origins{Unknown: 1}, nil
	case "provider_reported":
		return Origins{ProviderReported: 1}, nil
	case "inferred":
		return Origins{Inferred: 1}, nil
	case "unavailable":
		return Origins{Unavailable: 1}, nil
	}
	return Origins{}, fail("structure")
}

// AddAttempt stages one accepted attempt into a retained scalar row. Caller
// checks base updates and the window's model/byte budget before committing both.
// No input is mutated. Status, deduplication and acceptance remain with the base
// producer; unavailable provenance must already have suppressed its component.
func AddAttempt(previous Model, a Attempt) (Model, error) {
	row := Model{Requests: 1}
	var err error
	row.Provenance.Input, err = origin(a.InputOrigin)
	if err != nil {
		return Model{}, err
	}
	row.Provenance.Output, err = origin(a.OutputOrigin)
	if err != nil {
		return Model{}, err
	}
	obs := [4]Observation{a.Input, a.Output, a.CacheRead, a.CacheWrite}
	fs := [4]*Field{&row.Fields.Input, &row.Fields.Output, &row.Fields.CacheRead, &row.Fields.CacheWrite}
	invalid, missing := false, false
	var values [4]uint64
	for i, o := range obs {
		f := fs[i]
		f.Invalid = flag(o.Invalid)
		f.Conflict = flag(o.Conflict)
		f.SubsetViolation = flag(o.SubsetViolation)
		invalid = invalid || o.Invalid || o.Conflict || o.SubsetViolation
		if o.Value == nil {
			f.Missing = 1
			missing = true
		} else {
			if *o.Value > Max {
				return Model{}, fail("range")
			}
			f.Reported = 1
			f.Tokens = *o.Value
			values[i] = *o.Value
		}
	}
	i, o, r, w := values[0], values[1], values[2], values[3]
	if a.Input.Value != nil && a.CacheRead.Value != nil && a.CacheWrite.Value != nil && (r > i || w > i-r) {
		row.CachePartitionViolations = 1
		invalid = true
	}
	bin := 0
	if a.Input.Value != nil {
		for i > edges[bin] {
			bin++
		}
		row.Histogram[bin].Reported = Reported{Requests: 1, InputTokens: i}
	}
	switch {
	case invalid:
		row.Excluded.InvalidUsage = 1
	case missing:
		row.Excluded.MissingSplit = 1
	case i == 0 && o == 0 && (row.Provenance.Input.Unknown != 0 || row.Provenance.Output.Unknown != 0):
		row.Excluded.ZeroOriginUnknown = 1
	default:
		row.Complete = Cohort{Requests: 1, InputTokens: i, OutputTokens: o, CacheReadInputTokens: r, CacheWriteInputTokens: w, InputMin: &i, InputMax: &i}
		row.Histogram[bin].Complete = row.Complete
	}
	return Add(previous, row)
}
