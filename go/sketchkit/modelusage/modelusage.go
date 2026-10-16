// SPDX-License-Identifier: Apache-2.0

// Package modelusage implements the optional Model Usage v1 summary extension.
// It does not change base acceptance, identity normalization, routing, or prices.
package modelusage

import (
	"math"
	"math/big"
	"regexp"
)

const MaxModels = 5000
const HistogramID = "input-tiers-v1"
const Max = uint64(math.MaxInt64)

var publicID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$`)
var contractID = regexp.MustCompile(`^model-usage-v1:[0-9a-f]{64}$`)
var edges = [4]uint64{100000, 200000, 272000, Max}

// Error contains only a static, product-neutral conformance reason.
type Error struct{ Reason string }

func (e *Error) Error() string { return "invalid model_usage: " + e.Reason }
func fail(reason string) error { return &Error{Reason: reason} }

func IsPublicID(id string) bool { return publicID.MatchString(id) }
func IsReserved(id string) bool {
	switch id {
	case "__overflow__", "__missing__", "__too_long__", "__unmapped__":
		return true
	}
	return false
}

// Struct fields follow lexical wire order. Empty enabled blocks have Models:{}.
type Block struct {
	ContractID  string           `json:"contract_id"`
	HistogramID string           `json:"histogram_id"`
	Models      map[string]Model `json:"models"`
	Version     uint64           `json:"version"`
}
type Model struct {
	CachePartitionViolations uint64     `json:"cache_partition_violations"`
	Complete                 Cohort     `json:"complete"`
	Excluded                 Exclusions `json:"excluded"`
	Fields                   Fields     `json:"fields"`
	Histogram                [4]Bin     `json:"histogram"`
	Provenance               Provenance `json:"provenance"`
	Requests                 uint64     `json:"requests"`
}
type Field struct {
	Conflict        uint64 `json:"conflict"`
	Invalid         uint64 `json:"invalid"`
	Missing         uint64 `json:"missing"`
	Reported        uint64 `json:"reported"`
	SubsetViolation uint64 `json:"subset_violation"`
	Tokens          uint64 `json:"tokens"`
}
type Fields struct {
	CacheRead  Field `json:"cache_read"`
	CacheWrite Field `json:"cache_write"`
	Input      Field `json:"input"`
	Output     Field `json:"output"`
}
type Cohort struct {
	CacheReadInputTokens  uint64  `json:"cache_read_input_tokens"`
	CacheWriteInputTokens uint64  `json:"cache_write_input_tokens"`
	InputMax              *uint64 `json:"input_max"`
	InputMin              *uint64 `json:"input_min"`
	InputTokens           uint64  `json:"input_tokens"`
	OutputTokens          uint64  `json:"output_tokens"`
	Requests              uint64  `json:"requests"`
}
type Reported struct {
	InputTokens uint64 `json:"input_tokens"`
	Requests    uint64 `json:"requests"`
}
type Bin struct {
	Complete Cohort   `json:"complete"`
	Reported Reported `json:"reported"`
}
type Exclusions struct {
	InvalidUsage      uint64 `json:"invalid_usage"`
	MissingSplit      uint64 `json:"missing_split"`
	ZeroOriginUnknown uint64 `json:"zero_origin_unknown"`
}
type Origins struct {
	Inferred         uint64 `json:"inferred"`
	ProviderReported uint64 `json:"provider_reported"`
	Unavailable      uint64 `json:"unavailable"`
	Unknown          uint64 `json:"unknown"`
}
type Provenance struct {
	Input  Origins `json:"input"`
	Output Origins `json:"output"`
}

func New(contract string) (*Block, error) {
	b := &Block{ContractID: contract, HistogramID: HistogramID, Version: 1, Models: map[string]Model{}}
	if err := b.Validate(nil, false); err != nil {
		return nil, err
	}
	return b, nil
}

// CheckNames checks the real-model budget before inspecting or allocating rows.
func (b *Block) CheckNames(maxModels int) error {
	if b == nil || b.Models == nil {
		return fail("structure")
	}
	if maxModels < 1 || maxModels > MaxModels {
		return fail("range")
	}
	if len(b.Models) > MaxModels+4 {
		return fail("model_count")
	}
	n := 0
	for id := range b.Models {
		if !IsReserved(id) {
			n++
		}
	}
	if n > maxModels {
		return fail("model_count")
	}
	for id := range b.Models {
		if !IsReserved(id) && !IsPublicID(id) {
			return fail("structure")
		}
	}
	return nil
}

func inRange(values ...uint64) bool {
	for _, v := range values {
		if v > Max {
			return false
		}
	}
	return true
}
func sum(values ...uint64) (uint64, error) {
	var result uint64
	for _, v := range values {
		if v > Max || result > Max-v {
			return 0, fail("overflow")
		}
		result += v
	}
	return result, nil
}
func equals(total uint64, values ...uint64) bool {
	n, err := sum(values...)
	return err == nil && n == total
}
func totals(c Cohort) [5]uint64 {
	return [5]uint64{c.Requests, c.InputTokens, c.OutputTokens, c.CacheReadInputTokens, c.CacheWriteInputTokens}
}
func fieldList(f Fields) [4]Field { return [4]Field{f.Input, f.Output, f.CacheRead, f.CacheWrite} }

func (c Cohort) validate() error {
	t := totals(c)
	if !inRange(t[:]...) || (c.InputMin != nil && *c.InputMin > Max) || (c.InputMax != nil && *c.InputMax > Max) {
		return fail("range")
	}
	if c.Requests == 0 {
		if c.InputMin != nil || c.InputMax != nil || t != [5]uint64{} {
			return fail("conservation")
		}
	} else {
		if c.InputMin == nil || c.InputMax == nil || *c.InputMin > *c.InputMax {
			return fail("conservation")
		}
		// Attainable extrema subsume the ordinary count*min/max bounds.
		n := new(big.Int).SetUint64(c.Requests - 1)
		lo, hi := new(big.Int).SetUint64(*c.InputMin), new(big.Int).SetUint64(*c.InputMax)
		lower := new(big.Int).Add(new(big.Int).Mul(n, lo), hi)
		upper := new(big.Int).Add(new(big.Int).Mul(n, hi), lo)
		v := new(big.Int).SetUint64(c.InputTokens)
		if v.Cmp(lower) < 0 || v.Cmp(upper) > 0 {
			return fail("conservation")
		}
	}
	if c.CacheReadInputTokens > c.InputTokens || c.CacheWriteInputTokens > c.InputTokens-c.CacheReadInputTokens {
		return fail("conservation")
	}
	return nil
}

// Validate checks one nonempty retained model row.
func (m Model) Validate() error {
	if !inRange(m.Requests, m.CachePartitionViolations) {
		return fail("range")
	}
	if m.Requests == 0 || m.CachePartitionViolations > m.Requests {
		return fail("conservation")
	}
	for i, f := range fieldList(m.Fields) {
		if !inRange(f.Tokens, f.Reported, f.Missing, f.Invalid, f.Conflict, f.SubsetViolation) {
			return fail("range")
		}
		if !equals(m.Requests, f.Reported, f.Missing) || f.Invalid > m.Requests || f.Conflict > m.Requests || f.SubsetViolation > m.Requests || (f.Reported == 0 && f.Tokens != 0) || (i < 2 && f.SubsetViolation != 0) {
			return fail("conservation")
		}
	}
	if err := m.Complete.validate(); err != nil {
		return err
	}
	x := m.Excluded
	if !inRange(x.InvalidUsage, x.MissingSplit, x.ZeroOriginUnknown) {
		return fail("range")
	}
	if !equals(m.Requests, m.Complete.Requests, x.InvalidUsage, x.MissingSplit, x.ZeroOriginUnknown) || x.InvalidUsage < m.CachePartitionViolations {
		return fail("conservation")
	}
	t := totals(m.Complete)
	for i, f := range fieldList(m.Fields) {
		if m.Complete.Requests > f.Reported || t[i+1] > f.Tokens {
			return fail("conservation")
		}
		// Equal populations cannot retain an unaccounted component remainder.
		if m.Complete.Requests == f.Reported && t[i+1] != f.Tokens {
			return fail("conservation")
		}
	}
	for i, p := range [2]Origins{m.Provenance.Input, m.Provenance.Output} {
		if !inRange(p.ProviderReported, p.Inferred, p.Unavailable, p.Unknown) {
			return fail("range")
		}
		f := fieldList(m.Fields)[i]
		known, err := sum(p.ProviderReported, p.Inferred)
		if !equals(m.Requests, p.ProviderReported, p.Inferred, p.Unavailable, p.Unknown) || p.Unavailable > f.Missing || err != nil || known > f.Reported {
			return fail("conservation")
		}
	}
	var reportedN, reportedT uint64
	var complete Cohort
	low := uint64(0)
	for i, b := range m.Histogram {
		if !inRange(b.Reported.Requests, b.Reported.InputTokens) {
			return fail("range")
		}
		if err := b.Complete.validate(); err != nil {
			return err
		}
		n := new(big.Int).SetUint64(b.Reported.Requests)
		v := new(big.Int).SetUint64(b.Reported.InputTokens)
		if v.Cmp(new(big.Int).Mul(n, new(big.Int).SetUint64(low))) < 0 || v.Cmp(new(big.Int).Mul(n, new(big.Int).SetUint64(edges[i]))) > 0 {
			return fail("conservation")
		}
		if b.Complete.Requests > b.Reported.Requests || b.Complete.InputTokens > b.Reported.InputTokens {
			return fail("conservation")
		}
		if b.Complete.Requests == b.Reported.Requests && b.Complete.InputTokens != b.Reported.InputTokens {
			return fail("conservation")
		}
		if b.Complete.Requests > 0 && (*b.Complete.InputMin < low || *b.Complete.InputMax > edges[i]) {
			return fail("conservation")
		}
		var err error
		reportedN, err = sum(reportedN, b.Reported.Requests)
		if err != nil {
			return fail("conservation")
		}
		reportedT, err = sum(reportedT, b.Reported.InputTokens)
		if err != nil {
			return fail("conservation")
		}
		complete, err = addCohort(complete, b.Complete)
		if err != nil {
			return fail("conservation")
		}
		low = edges[i] + 1
	}
	if reportedN != m.Fields.Input.Reported || reportedT != m.Fields.Input.Tokens || totals(complete) != totals(m.Complete) || !samePtr(complete.InputMin, m.Complete.InputMin) || !samePtr(complete.InputMax, m.Complete.InputMax) {
		return fail("conservation")
	}
	return nil
}

// Validate optionally checks exact conservation against the five base counters.
func (b *Block) Validate(counters map[string]uint64, instantaneous bool) error {
	if err := b.CheckNames(MaxModels); err != nil {
		return err
	}
	if b.Version > Max {
		return fail("range")
	}
	if b.Version != 1 || b.HistogramID != HistogramID {
		return fail("unsupported_extension")
	}
	if !contractID.MatchString(b.ContractID) {
		return fail("structure")
	}
	if instantaneous && len(b.Models) > 0 {
		return fail("interval")
	}
	var sums [5]uint64
	for _, m := range b.Models {
		if err := m.Validate(); err != nil {
			return err
		}
		v := [5]uint64{m.Requests, m.Fields.Input.Tokens, m.Fields.Output.Tokens, m.Fields.CacheRead.Tokens, m.Fields.CacheWrite.Tokens}
		for i := range sums {
			var err error
			sums[i], err = sum(sums[i], v[i])
			if err != nil {
				return fail("conservation")
			}
		}
	}
	if counters != nil {
		for i, k := range []string{"requests", "input_tokens", "output_tokens", "cache_read_input_tokens", "cache_write_input_tokens"} {
			v, ok := counters[k]
			if !ok {
				return fail("structure")
			}
			if v != sums[i] {
				return fail("conservation")
			}
		}
	}
	return nil
}

func samePtr(a, b *uint64) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
func extremum(a, b *uint64, minimum bool) *uint64 {
	if a == nil && b == nil {
		return nil
	}
	var v uint64
	if a == nil {
		v = *b
	} else if b == nil {
		v = *a
	} else if minimum {
		v = min(*a, *b)
	} else {
		v = max(*a, *b)
	}
	return &v
}
func addCohort(a, b Cohort) (Cohort, error) {
	t, u := totals(a), totals(b)
	for i := range t {
		var err error
		t[i], err = sum(t[i], u[i])
		if err != nil {
			return Cohort{}, err
		}
	}
	return Cohort{Requests: t[0], InputTokens: t[1], OutputTokens: t[2], CacheReadInputTokens: t[3], CacheWriteInputTokens: t[4], InputMin: extremum(a.InputMin, b.InputMin, true), InputMax: extremum(a.InputMax, b.InputMax, false)}, nil
}

// Compatible checks extension presence and the full declared contract.
func Compatible(a, b *Block) error {
	if (a == nil) != (b == nil) {
		return fail("presence")
	}
	if a != nil && (a.ContractID != b.ContractID || a.HistogramID != b.HistogramID || a.Version != b.Version) {
		return fail("contract")
	}
	return nil
}
