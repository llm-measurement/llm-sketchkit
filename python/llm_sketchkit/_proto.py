# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay and Codex
"""Small typed wrappers around generated protobuf classes."""

from __future__ import annotations

from typing import Any, cast

from . import sketches_pb2 as _sketchpb

sketchpb: Any = _sketchpb

WIRE_VERSION = 1
MAX_WIRE_BYTES = 4 * 1024 * 1024
SMALL_WIRE_BYTES = 64 << 10
MINHASH_WIRE_BYTES = 4 << 10
BLOOM_WIRE_BYTES = 5 << 19

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


def _varint(data: bytes | memoryview, pos: int) -> tuple[int, int]:
    value = 0
    for shift in range(0, 70, 7):
        if pos >= len(data):
            raise ValueError
        byte = data[pos]
        pos += 1
        value |= (byte & 127) << shift
        if byte < 128:
            if shift == 63 and byte > 1:
                raise ValueError
            return value, pos
    raise ValueError


def bounded_wire(data: bytes, limit: int) -> bool:
    """Bound discarded oneof bodies before protobuf allocates their entries."""
    if len(data) > min(limit, MAX_WIRE_BYTES):
        return False
    if not _no_groups(memoryview(data), 0):
        return False
    sizes = [0, 0, 0, 0]
    messages = 0
    entries = 0
    caps = (SMALL_WIRE_BYTES, SMALL_WIRE_BYTES, BLOOM_WIRE_BYTES,
            MINHASH_WIRE_BYTES)
    pos = 0

    def varint(offset: int) -> tuple[int, int]:
        return _varint(data, offset)

    groups: list[int] = []
    try:
        while pos < len(data):
            tag, pos = varint(pos)
            field, wire_type = tag >> 3, tag & 7
            if not 0 < field < 1 << 29:
                return False
            if wire_type == 0:
                _, pos = varint(pos)
            elif wire_type == 1:
                pos += 8
            elif wire_type == 2:
                size, pos = varint(pos)
                if not groups and (field == 1 or 10 <= field <= 13):
                    messages += 1
                    if messages > 4096:
                        return False
                if not groups and 10 <= field <= 13:
                    sizes[field - 10] += size + 1
                    if sizes[field - 10] > caps[field - 10]:
                        return False
                    if field in (10, 11):
                        entries += _entry_count(memoryview(data)[pos:pos + size])
                        if entries > 4096:
                            return False
                pos += size
            elif wire_type == 3:
                groups.append(field)
            elif wire_type == 4:
                if not groups or groups.pop() != field:
                    return False
            elif wire_type == 5:
                pos += 4
            else:
                return False
            if pos > len(data):
                return False
    except ValueError:
        return False
    return not groups


def _no_groups(data: memoryview, message: int) -> bool:
    """Follow only schema-defined submessages, never opaque byte fields."""
    pos = 0

    def varint() -> int:
        nonlocal pos
        value = 0
        for shift in range(0, 70, 7):
            if pos == len(data):
                raise ValueError
            byte = data[pos]
            pos += 1
            value |= (byte & 127) << shift
            if byte < 128:
                if shift == 63 and byte > 1:
                    raise ValueError
                return value
        raise ValueError

    try:
        while pos < len(data):
            tag = varint()
            field, wire_type = tag >> 3, tag & 7
            if not 0 < field < 1 << 29:
                return False
            if wire_type == 0:
                varint()
            elif wire_type == 1:
                pos += 8
            elif wire_type == 2:
                size = varint()
                end = pos + size
                if end > len(data):
                    return False
                child = -1
                if message == 0 and (field == 1 or 10 <= field <= 13):
                    child = field
                elif message in (10, 11) and field == 1:
                    child = 100
                if child >= 0 and not _no_groups(data[pos:end], child):
                    return False
                pos = end
            elif wire_type == 5:
                pos += 4
            else:
                return False
            if pos > len(data):
                return False
    except ValueError:
        return False
    return True


def _entry_count(data: memoryview) -> int:
    """Share an allocation budget across both repeated-message bodies."""
    pos = count = 0
    groups: list[int] = []

    def varint() -> int:
        nonlocal pos
        value, pos = _varint(data, pos)
        return value

    while pos < len(data):
        tag = varint()
        field, wire_type = tag >> 3, tag & 7
        if not 0 < field < 1 << 29:
            raise ValueError
        if wire_type == 0:
            varint()
        elif wire_type == 1:
            pos += 8
        elif wire_type == 2:
            size = varint()
            if not groups and field == 1:
                count += 1
                if count > 4096:
                    raise ValueError
            pos += size
        elif wire_type == 3:
            groups.append(field)
        elif wire_type == 4:
            if not groups or groups.pop() != field:
                raise ValueError
        elif wire_type == 5:
            pos += 4
        else:
            raise ValueError
        if pos > len(data):
            raise ValueError
    if groups:
        raise ValueError
    return count
