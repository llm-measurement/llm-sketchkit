# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay Erramilli and Codex
"""Small typed wrappers around generated protobuf classes."""

from __future__ import annotations

from typing import Any, cast

from . import sketches_pb2 as _sketchpb

sketchpb: Any = _sketchpb

WIRE_VERSION = 1
MAX_WIRE_BYTES = 4 * 1024 * 1024

HASH_ALGORITHM_HMAC_SHA256_64: int = sketchpb.HASH_ALGORITHM_HMAC_SHA256_64
SKETCH_KIND_HLLPP: int = sketchpb.SKETCH_KIND_HLLPP
SKETCH_KIND_FREQUENT_ITEMS: int = sketchpb.SKETCH_KIND_FREQUENT_ITEMS
SKETCH_KIND_BLOOM: int = sketchpb.SKETCH_KIND_BLOOM
SKETCH_KIND_MINHASH: int = sketchpb.SKETCH_KIND_MINHASH
REPRESENTATION_HLLPP_SPARSE: int = sketchpb.REPRESENTATION_MODE_HLLPP_SPARSE
REPRESENTATION_HLLPP_DENSE: int = sketchpb.REPRESENTATION_MODE_HLLPP_DENSE
REPRESENTATION_FREQUENT_ITEMS_BOUNDED_MAP: int = (
    sketchpb.REPRESENTATION_MODE_FREQUENT_ITEMS_BOUNDED_MAP
)
REPRESENTATION_BLOOM_BITSET: int = sketchpb.REPRESENTATION_MODE_BLOOM_BITSET
REPRESENTATION_MINHASH_SIGNATURE: int = sketchpb.REPRESENTATION_MODE_MINHASH_SIGNATURE


def serialize(message: Any) -> bytes:
    return cast(bytes, message.SerializeToString(deterministic=True))


def parse_sketch(data: bytes) -> Any:
    message = sketchpb.Sketch()
    message.ParseFromString(data)
    return message
