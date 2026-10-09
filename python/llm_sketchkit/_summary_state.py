# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay and Codex
"""Operation-local parsed state for summary validation and accumulation."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any

from . import _proto, bloom, frequentitems, hllpp, minhash

State = hllpp.Sketch | frequentitems.Sketch | bloom.Sketch | minhash.Sketch


@dataclass
class Parsed:
    state: State
    metadata: Any

    def empty(self) -> bool:
        state = self.state
        if isinstance(state, hllpp.Sketch):
            return state.sparse_count() == 0 and state.dense_nonzero_count() == 0
        if isinstance(state, frequentitems.Sketch):
            return state.total_weight() == 0
        if isinstance(state, bloom.Sketch):
            return state.inserted_count() == 0 and state.set_bit_count() == 0
        return state.populated_count() == 0

    def merge(self, other: Parsed) -> None:
        left, right = self.state, other.state
        if isinstance(left, hllpp.Sketch) and isinstance(right, hllpp.Sketch):
            left.merge(right)
        elif isinstance(left, frequentitems.Sketch) and isinstance(
            right, frequentitems.Sketch
        ):
            if right.total_weight():
                left.merge(right)
        elif isinstance(left, bloom.Sketch) and isinstance(right, bloom.Sketch):
            left.merge(right)
        elif isinstance(left, minhash.Sketch) and isinstance(right, minhash.Sketch):
            left.merge(right)
        else:
            raise ValueError("sketch kinds differ")


def decode(kind: str, data: bytes) -> State:
    if kind == "hllpp":
        return hllpp.parse(data)
    if kind == "frequent_items":
        return frequentitems.parse(data)
    if kind == "bloom":
        return bloom.parse(data)
    if kind == "minhash":
        return minhash.parse(data)
    raise ValueError("unknown summary sketch kind")


def parse(kind: str, data: bytes) -> tuple[Parsed, bytes]:
    state = decode(kind, data)
    canonical = state.marshal_binary()
    # Detach the header so retaining it cannot keep a decoded body alive.
    metadata = _proto.sketchpb.SketchMetadata()
    metadata.CopyFrom(_proto.parse_sketch(canonical).metadata)
    metadata.ClearField("representation_mode")
    return Parsed(state, metadata), canonical
