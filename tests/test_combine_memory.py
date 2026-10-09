# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay and Codex
from __future__ import annotations

import copy
import weakref
from dataclasses import replace

import pytest
from llm_sketchkit import _summary_state, frequentitems, summary


def memory_envelope() -> summary.Envelope:
    sketch = frequentitems.Sketch("micro")
    for value in range(256):
        sketch.add_hash(value, 1)
    payload = summary.Payload(sketch.marshal_binary(), "frequent_items")
    return summary.Envelope(
        accounting_id="test", counters={"requests": 0}, emitted_at_unix_nano=64,
        epoch="e000", key_id="key", observed_start_unix_nano=0,
        observed_end_unix_nano=1, producer_id="a", scope_id="test", sequence=1,
        sketches={f"s{i:02d}": payload for i in range(16)}, version=1,
        window_duration_unix_nano=64, window_start_unix_nano=0,
    )


@pytest.mark.parametrize("mode", ["duplicates", "epochs", "superseded"])
def test_combine_bounds_live_states(mode: str, monkeypatch: pytest.MonkeyPatch) -> None:
    doc = memory_envelope()
    if mode == "duplicates":
        docs = [doc] * 32
    elif mode == "epochs":
        docs = [replace(doc, epoch=f"e{i:03d}", observed_start_unix_nano=i,
                        observed_end_unix_nano=i + 1) for i in reversed(range(32))]
    else:
        docs = [replace(doc, sequence=i + 1) for i in reversed(range(32))]
    before = copy.deepcopy(docs)
    live: weakref.WeakSet[frequentitems.Sketch] = weakref.WeakSet()
    peak = validations = canonicalizations = marshals = 0
    parse = frequentitems.parse
    validate = summary.Envelope._validate
    canonicalize = summary.Envelope._marshal_validated
    marshal = frequentitems.Sketch.marshal_binary

    def tracked_parse(data: bytes) -> frequentitems.Sketch:
        nonlocal peak
        state = parse(data)
        live.add(state)
        peak = max(peak, len(live))
        return state

    def tracked_validate(self: summary.Envelope) -> dict[str, _summary_state.Parsed]:
        nonlocal validations
        validations += 1
        return validate(self)

    def tracked_canonicalize(self: summary.Envelope) -> bytes:
        nonlocal canonicalizations
        canonicalizations += 1
        return canonicalize(self)

    def tracked_marshal(self: frequentitems.Sketch) -> bytes:
        nonlocal marshals
        marshals += 1
        return marshal(self)

    monkeypatch.setattr(frequentitems, "parse", tracked_parse)
    monkeypatch.setattr(summary.Envelope, "_validate", tracked_validate)
    monkeypatch.setattr(summary.Envelope, "_marshal_validated", tracked_canonicalize)
    monkeypatch.setattr(frequentitems.Sketch, "marshal_binary", tracked_marshal)
    result = summary.combine(docs, ["a"])
    assert validations == canonicalizations == len(docs)
    assert marshals == len(docs) * 16 + 16
    assert peak <= 48, f"retained {peak} decoded states (three-envelope budget)"
    assert len(result.sources) == (32 if mode == "epochs" else 1)
    assert docs == before


def test_input_validation_precedes_sorted_selection() -> None:
    doc = memory_envelope()
    incompatible = replace(doc, key_id="other", producer_id="b")
    invalid = replace(doc, version=0, producer_id="z")
    with pytest.raises(
        summary.SummaryError, match="^invalid summary version or sequence$"
    ):
        summary.combine([doc, incompatible, invalid], ["a", "b", "z"])
    invalid_identifier = replace(invalid, version=1, epoch="bad identifier")
    with pytest.raises(summary.SummaryError, match="^invalid summary identifier$"):
        summary.combine([invalid_identifier, invalid], ["a", "b", "z"])
