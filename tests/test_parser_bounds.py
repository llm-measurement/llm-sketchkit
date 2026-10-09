# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay and Codex
"""Allocation and documented-error regression cases for peer-supplied bytes."""

import tracemalloc

import pytest
from llm_sketchkit import _proto, bloom, frequentitems, hllpp, minhash, summary


@pytest.mark.parametrize("module", [bloom, frequentitems, hllpp, minhash])
def test_discarded_body_is_bounded(module):  # type: ignore[no-untyped-def]
    message = _proto.sketchpb.Sketch()
    for _ in range(40000):
        message.frequent_items.entries.add()
    data = message.SerializeToString()
    tracemalloc.start()
    try:
        with pytest.raises(module.InvalidWireEncodingError):
            module.parse(data)
        assert tracemalloc.get_traced_memory()[1] < 8 << 20
    finally:
        tracemalloc.stop()


@pytest.mark.parametrize("module", [bloom, frequentitems, hllpp, minhash])
def test_malformed_protobuf_has_documented_error(module):  # type: ignore[no-untyped-def]
    with pytest.raises(module.InvalidWireEncodingError):
        module.parse(b"\x0a\x01\xff")


def test_empty_oneof_amplification() -> None:
    data = b"\x52\x00\x5a\x00" * 20000
    with pytest.raises(bloom.InvalidWireEncodingError):
        bloom.parse(data)


def test_mixed_body_allocation_budget() -> None:
    body = b"\x0a\x00" * 32760
    data = (b"\x52\xf0\xff\x03" + body + b"\x5a\xf0\xff\x03" + body
            + bloom.Sketch("default").marshal_binary())
    tracemalloc.start()
    try:
        with pytest.raises(bloom.InvalidWireEncodingError):
            bloom.parse(data)
        assert tracemalloc.get_traced_memory()[1] < 8 << 20
    finally:
        tracemalloc.stop()


@pytest.mark.parametrize("key", ["counters", "sketches"])
def test_summary_map_count_precedes_materialization(key: str) -> None:
    data = (f'{{"{key}":{{' + '"a":0,' * 500000 + '"z":0}}').encode()
    tracemalloc.start()
    try:
        with pytest.raises(summary.SummaryError):
            summary.Envelope.parse(data)
        assert tracemalloc.get_traced_memory()[1] < len(data) + (1 << 20)
    finally:
        tracemalloc.stop()


@pytest.mark.parametrize("module", [bloom, frequentitems, hllpp, minhash])
@pytest.mark.parametrize("profile", ["micro", "small", "default"])
def test_full_profile_roundtrip(module, profile: str):  # type: ignore[no-untyped-def]
    sketch = module.Sketch(profile)
    if module is hllpp:
        for key in range(50000):
            sketch.add_hash((key * 0x9E3779B97F4A7C15) & ((1 << 64) - 1))
    elif module is frequentitems:
        for key in range(sketch.map_size()):
            sketch.add_hash(key, 1 << 40)
    wire = sketch.marshal_binary()
    assert module.parse(wire).marshal_binary() == wire


def test_minhash_largest_signature_roundtrip() -> None:
    wire = minhash.Sketch("k256").marshal_binary()
    assert minhash.parse(wire).marshal_binary() == wire
