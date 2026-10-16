// SPDX-License-Identifier: Apache-2.0

package modelusage

// scalarPointers enumerates only additive state, never extrema or contracts.
func scalarPointers(m *Model) []*uint64 {
	p := []*uint64{&m.Requests, &m.CachePartitionViolations, &m.Excluded.InvalidUsage, &m.Excluded.MissingSplit, &m.Excluded.ZeroOriginUnknown}
	for _, f := range []*Field{&m.Fields.Input, &m.Fields.Output, &m.Fields.CacheRead, &m.Fields.CacheWrite} {
		p = append(p, &f.Tokens, &f.Reported, &f.Missing, &f.Invalid, &f.Conflict, &f.SubsetViolation)
	}
	for _, o := range []*Origins{&m.Provenance.Input, &m.Provenance.Output} {
		p = append(p, &o.ProviderReported, &o.Inferred, &o.Unavailable, &o.Unknown)
	}
	for _, c := range cohorts(m) {
		p = append(p, &c.Requests, &c.InputTokens, &c.OutputTokens, &c.CacheReadInputTokens, &c.CacheWriteInputTokens)
	}
	for i := range m.Histogram {
		p = append(p, &m.Histogram[i].Reported.Requests, &m.Histogram[i].Reported.InputTokens)
	}
	return p
}
func cohorts(m *Model) []*Cohort {
	return []*Cohort{&m.Complete, &m.Histogram[0].Complete, &m.Histogram[1].Complete, &m.Histogram[2].Complete, &m.Histogram[3].Complete}
}

// Add combines two disjoint, valid rows. A zero Model is an empty accumulator.
// Returned extrema are newly owned; neither input is modified, even on overflow.
func Add(a, b Model) (Model, error) {
	for _, m := range []Model{a, b} {
		if m.Requests != 0 {
			if err := m.Validate(); err != nil {
				return Model{}, err
			}
		} else if m != (Model{}) {
			return Model{}, fail("conservation")
		}
	}
	return addValidated(a, b)
}
func addValidated(a, b Model) (Model, error) {
	result := a
	x, y := scalarPointers(&result), scalarPointers(&b)
	for i := range x {
		v, err := sum(*x[i], *y[i])
		if err != nil {
			return Model{}, err
		}
		*x[i] = v
	}
	for i, c := range cohorts(&result) {
		d := cohorts(&b)[i]
		c.InputMin = extremum(c.InputMin, d.InputMin, true)
		c.InputMax = extremum(c.InputMax, d.InputMax, false)
	}
	return result, nil
}

// CheckRegression checks cumulative replacement, including per-bin history.
func CheckRegression(a, b *Block) error {
	if err := Compatible(a, b); err != nil {
		return err
	}
	if a == nil {
		return nil
	}
	for id, old := range a.Models {
		newer, ok := b.Models[id]
		if !ok {
			return fail("regression")
		}
		x, y := scalarPointers(&old), scalarPointers(&newer)
		for i := range x {
			if *y[i] < *x[i] {
				return fail("regression")
			}
		}
		for i, c := range cohorts(&old) {
			d := cohorts(&newer)[i]
			if c.InputMin != nil && (d.InputMin == nil || *d.InputMin > *c.InputMin) {
				return fail("regression")
			}
			if c.InputMax != nil && (d.InputMax == nil || *d.InputMax < *c.InputMax) {
				return fail("regression")
			}
		}
	}
	return nil
}

// Merge combines disjoint owners, not cumulative snapshots. Summary.Combine
// performs replay/replacement selection before calling this function.
func Merge(a, b *Block) (*Block, error) {
	if b == nil {
		return nil, fail("structure")
	}
	if err := b.Validate(nil, false); err != nil {
		return nil, err
	}
	if a != nil {
		if err := a.Validate(nil, false); err != nil {
			return nil, err
		}
		if err := Compatible(a, b); err != nil {
			return nil, err
		}
	}
	// Count the union before allocating row state. Reserved names have fixed slots.
	real := 0
	for k := range b.Models {
		if !IsReserved(k) {
			real++
		}
	}
	if a != nil {
		for k := range a.Models {
			if _, ok := b.Models[k]; !ok && !IsReserved(k) {
				real++
			}
		}
	}
	if real > MaxModels {
		return nil, fail("model_count")
	}
	out := &Block{ContractID: b.ContractID, HistogramID: b.HistogramID, Version: b.Version, Models: map[string]Model{}}
	if a != nil {
		for k, m := range a.Models {
			cloned, err := addValidated(Model{}, m)
			if err != nil {
				return nil, err
			}
			out.Models[k] = cloned
		}
	}
	for k, m := range b.Models {
		merged, err := addValidated(out.Models[k], m)
		if err != nil {
			return nil, err
		}
		out.Models[k] = merged
	}
	// Disjoint model names can overflow base totals without overflowing a row.
	var totals [5]uint64
	for _, m := range out.Models {
		values := [5]uint64{m.Requests, m.Fields.Input.Tokens, m.Fields.Output.Tokens, m.Fields.CacheRead.Tokens, m.Fields.CacheWrite.Tokens}
		for i, value := range values {
			var err error
			totals[i], err = sum(totals[i], value)
			if err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}
