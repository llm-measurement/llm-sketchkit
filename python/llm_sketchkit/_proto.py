# Code authors: Vijay Erramilli and Codex
"""Small typed wrappers around generated protobuf classes."""

from __future__ import annotations

import importlib
from typing import Any, cast

WIRE_VERSION = 1
MAX_WIRE_BYTES = 4 * 1024 * 1024
sketchpb: Any = importlib.import_module("spec.sketches_pb2")

__all__ = [
    "WIRE_VERSION",
    "MAX_WIRE_BYTES",
    "hash_algorithm_hmac_sha256_64",
    "parse_sketch",
    "representation_bloom_bitset",
    "representation_frequent_items_bounded_map",
    "representation_hllpp_dense",
    "representation_hllpp_sparse",
    "representation_minhash_signature",
    "serialize",
    "sketch_kind_bloom",
    "sketch_kind_frequent_items",
    "sketch_kind_hllpp",
    "sketch_kind_minhash",
    "sketchpb",
]


def serialize(message: Any) -> bytes:
    return cast(bytes, message.SerializeToString(deterministic=True))


def parse_sketch(data: bytes) -> Any:
    message = sketchpb.Sketch()
    message.ParseFromString(data)
    return message


def hash_algorithm_hmac_sha256_64() -> int:
    return cast(int, sketchpb.HASH_ALGORITHM_HMAC_SHA256_64)


def sketch_kind_hllpp() -> int:
    return cast(int, sketchpb.SKETCH_KIND_HLLPP)


def sketch_kind_frequent_items() -> int:
    return cast(int, sketchpb.SKETCH_KIND_FREQUENT_ITEMS)


def sketch_kind_bloom() -> int:
    return cast(int, sketchpb.SKETCH_KIND_BLOOM)


def sketch_kind_minhash() -> int:
    return cast(int, sketchpb.SKETCH_KIND_MINHASH)


def representation_hllpp_sparse() -> int:
    return cast(int, sketchpb.REPRESENTATION_MODE_HLLPP_SPARSE)


def representation_hllpp_dense() -> int:
    return cast(int, sketchpb.REPRESENTATION_MODE_HLLPP_DENSE)


def representation_frequent_items_bounded_map() -> int:
    return cast(int, sketchpb.REPRESENTATION_MODE_FREQUENT_ITEMS_BOUNDED_MAP)


def representation_bloom_bitset() -> int:
    return cast(int, sketchpb.REPRESENTATION_MODE_BLOOM_BITSET)


def representation_minhash_signature() -> int:
    return cast(int, sketchpb.REPRESENTATION_MODE_MINHASH_SIGNATURE)
