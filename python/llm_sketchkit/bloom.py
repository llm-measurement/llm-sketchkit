# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay Erramilli and Codex
"""Pure-Python Bloom sketch implementation."""

from __future__ import annotations

import math
from typing import Any, cast

from . import _proto, hashfamily, profiles

MAX_UINT64 = (1 << 64) - 1


class BloomError(ValueError):
    """Base class for Bloom failures."""


class UnknownProfileError(BloomError):
    """Raised when a profile is not supported."""


class InvalidShapeError(BloomError):
    """Raised when wire metadata uses a non-profile shape."""


class IncompatibleMergeError(BloomError):
    """Raised when merge metadata differs."""


class CountOverflowError(BloomError):
    """Raised when inserted count exceeds uint64."""


class InvalidWireEncodingError(BloomError):
    """Raised when serialized bytes are malformed."""


class Sketch:
    """Mergeable Bloom filter over pre-hashed unsigned 64-bit values."""

    def __init__(
        self,
        profile: str,
        domain: str = profiles.PROMPT_V1,
        algorithm: str = profiles.HMAC_SHA256_64,
    ) -> None:
        config = profiles.BLOOM_PROFILES.get(profile)
        if config is None:
            raise UnknownProfileError(profile)
        if domain not in profiles.REGISTERED_DOMAINS:
            raise IncompatibleMergeError(domain)
        if algorithm != profiles.HMAC_SHA256_64:
            raise IncompatibleMergeError(algorithm)

        self._profile = profile
        self._config = config
        self._domain = domain
        self._algorithm = algorithm
        self._bitset = bytearray(_byte_len(config.bit_count))
        self._inserted_count = 0

    @classmethod
    def parse(cls, data: bytes) -> Sketch:
        """Decode a Bloom sketch from deterministic protobuf bytes."""

        if len(data) > _proto.MAX_WIRE_BYTES:
            raise InvalidWireEncodingError("wire input too large")
        message = _proto.parse_sketch(data)
        metadata = message.metadata
        if not message.HasField("metadata") or not message.HasField("bloom"):
            raise InvalidWireEncodingError("missing Bloom metadata or body")
        if metadata.kind != _proto.SKETCH_KIND_BLOOM:
            raise InvalidWireEncodingError("wrong sketch kind")
        if metadata.wire_version != _proto.WIRE_VERSION:
            raise InvalidWireEncodingError("wrong wire version")
        if metadata.hash_algo != _proto.HASH_ALGORITHM_HMAC_SHA256_64:
            raise InvalidWireEncodingError("wrong hash algorithm")
        if metadata.representation_mode != _proto.REPRESENTATION_BLOOM_BITSET:
            raise InvalidWireEncodingError("wrong representation")

        profile = cast(str, metadata.profile)
        config = profiles.BLOOM_PROFILES.get(profile)
        if config is None:
            raise UnknownProfileError(profile)
        if (
            cast(int, metadata.bloom_bit_count) != config.bit_count
            or cast(int, metadata.bloom_hash_count) != config.hash_count
        ):
            raise InvalidShapeError(profile)
        if cast(str, metadata.hash_domain) not in profiles.REGISTERED_DOMAINS:
            raise IncompatibleMergeError("unregistered domain")

        bitset = cast(bytes, message.bloom.bitset)
        if len(bitset) != _byte_len(config.bit_count):
            raise InvalidWireEncodingError("wrong bitset length")
        if not _final_byte_padding_zero(bitset, config.bit_count):
            raise InvalidWireEncodingError("final byte padding bits set")

        sketch = cls(profile, cast(str, metadata.hash_domain), profiles.HMAC_SHA256_64)
        sketch._bitset = bytearray(bitset)
        sketch._inserted_count = cast(int, message.bloom.inserted_count)
        return sketch

    def add_hash(self, value: int) -> None:
        """Insert a pre-hashed unsigned 64-bit value.

        CountOverflowError leaves self unchanged. Runtime failures and inputs
        outside the documented types have no rollback guarantee.
        """

        if self._inserted_count == MAX_UINT64:
            raise CountOverflowError("inserted count")
        value &= hashfamily.MASK64
        for index in range(self._config.hash_count):
            self._set_bit(self._location(value, index))
        self._inserted_count += 1

    def may_contain_hash(self, value: int) -> bool:
        """Return whether the value may have been inserted."""

        value &= hashfamily.MASK64
        for index in range(self._config.hash_count):
            if not self._bit(self._location(value, index)):
                return False
        return True

    def merge(self, other: Sketch | None) -> None:
        """OR another Bloom sketch with identical metadata into this sketch.

        IncompatibleMergeError and CountOverflowError leave self unchanged.
        None is a no-op; a distinct source is never modified. Runtime failures,
        invalid private state, and concurrent use have no rollback guarantee.
        """

        if other is None:
            return
        if (
            self._profile != other._profile
            or self._config.bit_count != other._config.bit_count
            or self._config.hash_count != other._config.hash_count
            or self._domain != other._domain
            or self._algorithm != other._algorithm
        ):
            raise IncompatibleMergeError("metadata differs")
        if self._inserted_count > MAX_UINT64 - other._inserted_count:
            raise CountOverflowError("inserted count")
        self._bitset = bytearray(
            left | right
            for left, right in zip(self._bitset, other._bitset, strict=True)
        )
        self._inserted_count += other._inserted_count

    def false_positive_estimate(self) -> float:
        """Return the fill-ratio Bloom false-positive estimate."""

        fill_ratio = float(self.set_bit_count()) / float(self._config.bit_count)
        return math.pow(fill_ratio, float(self._config.hash_count))

    def inserted_count(self) -> int:
        """Return the number of AddHash calls observed."""

        return self._inserted_count

    def bit_count(self) -> int:
        """Return the configured bit count."""

        return self._config.bit_count

    def hash_count(self) -> int:
        """Return the configured hash count."""

        return self._config.hash_count

    def rated_insertions(self) -> int:
        """Return the profile's rated insertion count."""

        return self._config.rated_insertions

    def target_fpr(self) -> float:
        """Return the profile's target false-positive rate."""

        return self._config.target_fpr

    def set_bit_count(self) -> int:
        """Return the number of one bits in the serialized bitset."""

        return sum(byte.bit_count() for byte in self._bitset)

    def bitset(self) -> bytes:
        """Return the canonical bitset bytes."""

        return bytes(self._bitset)

    def set_bits(self) -> list[int]:
        """Return set bit positions in ascending order."""

        out: list[int] = []
        for byte_index, byte in enumerate(self._bitset):
            for bit in range(8):
                if byte & (1 << bit):
                    out.append(byte_index * 8 + bit)
        return out

    def clone(self) -> Sketch:
        """Return a deep copy of the sketch."""

        clone = type(self)(self._profile, self._domain, self._algorithm)
        clone._bitset = bytearray(self._bitset)
        clone._inserted_count = self._inserted_count
        return clone

    def marshal_binary(self) -> bytes:
        """Return deterministic protobuf bytes."""

        return _proto.serialize(self._to_proto())

    def _location(self, value: int, index: int) -> int:
        return hashfamily.bloom_hash(value, index) % self._config.bit_count

    def _set_bit(self, position: int) -> None:
        self._bitset[position // 8] |= 1 << (position % 8)

    def _bit(self, position: int) -> bool:
        return self._bitset[position // 8] & (1 << (position % 8)) != 0

    def _to_proto(self) -> Any:
        metadata = _proto.sketchpb.SketchMetadata(
            kind=_proto.SKETCH_KIND_BLOOM,
            wire_version=_proto.WIRE_VERSION,
            profile=self._profile,
            hash_domain=self._domain,
            hash_algo=_proto.HASH_ALGORITHM_HMAC_SHA256_64,
            representation_mode=_proto.REPRESENTATION_BLOOM_BITSET,
        )
        metadata.bloom_bit_count = self._config.bit_count
        metadata.bloom_hash_count = self._config.hash_count

        body = _proto.sketchpb.BloomSketch(
            bitset=bytes(self._bitset),
            inserted_count=self._inserted_count,
        )
        return _proto.sketchpb.Sketch(metadata=metadata, bloom=body)


def new(
    profile: str,
    domain: str = profiles.PROMPT_V1,
    algorithm: str = profiles.HMAC_SHA256_64,
) -> Sketch:
    """Construct an empty Bloom sketch."""

    return Sketch(profile, domain, algorithm)


def parse(data: bytes) -> Sketch:
    """Decode a Bloom sketch from deterministic protobuf bytes."""

    return Sketch.parse(data)


def _byte_len(bit_count: int) -> int:
    return (bit_count + 7) // 8


def _final_byte_padding_zero(bitset: bytes, bit_count: int) -> bool:
    remainder = bit_count % 8
    if remainder == 0 or not bitset:
        return True
    mask = 0xFF << remainder
    return bitset[-1] & mask == 0
