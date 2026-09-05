# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay Erramilli and Codex
"""Pure-Python MinHash sketch implementation."""

from __future__ import annotations

from typing import Any, cast

from . import _proto, hashfamily, profiles

MAX_UINT64 = (1 << 64) - 1


class MinHashError(ValueError):
    """Base class for MinHash failures."""


class UnknownProfileError(MinHashError):
    """Raised when a profile is not supported."""


class InvalidSignatureLengthError(MinHashError):
    """Raised when wire metadata uses a non-profile signature length."""


class IncompatibleMergeError(MinHashError):
    """Raised when merge metadata differs."""


class CountOverflowError(MinHashError):
    """Raised when populated count exceeds uint64."""


class InvalidWireEncodingError(MinHashError):
    """Raised when serialized bytes are malformed."""


class Sketch:
    """Mergeable MinHash signature over pre-hashed unsigned 64-bit values."""

    def __init__(
        self,
        profile: str,
        domain: str = profiles.PROMPT_V1,
        algorithm: str = profiles.HMAC_SHA256_64,
    ) -> None:
        length = profiles.MINHASH_SIGNATURE_LENGTHS.get(profile)
        if length is None:
            raise UnknownProfileError(profile)
        if domain not in profiles.REGISTERED_DOMAINS:
            raise IncompatibleMergeError(domain)
        if algorithm != profiles.HMAC_SHA256_64:
            raise IncompatibleMergeError(algorithm)

        self._profile = profile
        self._length = length
        self._domain = domain
        self._algorithm = algorithm
        self._signature = [MAX_UINT64] * length
        self._populated_count = 0

    @classmethod
    def parse(cls, data: bytes) -> Sketch:
        """Decode a MinHash sketch from deterministic protobuf bytes."""

        if len(data) > _proto.MAX_WIRE_BYTES:
            raise InvalidWireEncodingError("wire input too large")
        message = _proto.parse_sketch(data)
        metadata = message.metadata
        if not message.HasField("metadata") or not message.HasField("minhash"):
            raise InvalidWireEncodingError("missing MinHash metadata or body")
        if metadata.kind != _proto.SKETCH_KIND_MINHASH:
            raise InvalidWireEncodingError("wrong sketch kind")
        if metadata.wire_version != _proto.WIRE_VERSION:
            raise InvalidWireEncodingError("wrong wire version")
        if metadata.hash_algo != _proto.HASH_ALGORITHM_HMAC_SHA256_64:
            raise InvalidWireEncodingError("wrong hash algorithm")
        if metadata.representation_mode != _proto.REPRESENTATION_MINHASH_SIGNATURE:
            raise InvalidWireEncodingError("wrong representation")

        profile = cast(str, metadata.profile)
        length = profiles.MINHASH_SIGNATURE_LENGTHS.get(profile)
        if length is None:
            raise UnknownProfileError(profile)
        if cast(int, metadata.minhash_signature_length) != length:
            raise InvalidSignatureLengthError(profile)
        if cast(str, metadata.hash_domain) not in profiles.REGISTERED_DOMAINS:
            raise IncompatibleMergeError("unregistered domain")

        body = message.minhash
        signature = [cast(int, value) for value in body.signature]
        populated_count = cast(int, body.populated_count)
        if len(signature) != length:
            raise InvalidWireEncodingError("wrong signature length")
        if populated_count == 0 and any(value != MAX_UINT64 for value in signature):
            raise InvalidWireEncodingError("non-empty signature with zero count")

        sketch = cls(profile, cast(str, metadata.hash_domain), profiles.HMAC_SHA256_64)
        sketch._signature = signature
        sketch._populated_count = populated_count
        return sketch

    def add_hash(self, value: int) -> None:
        """Update the signature with a pre-hashed unsigned 64-bit value.

        CountOverflowError leaves self unchanged. Runtime failures and inputs
        outside the documented types have no rollback guarantee.
        """

        if self._populated_count == MAX_UINT64:
            raise CountOverflowError("populated count")
        value &= hashfamily.MASK64
        for index in range(self._length):
            candidate = hashfamily.minhash(value, index)
            if candidate < self._signature[index]:
                self._signature[index] = candidate
        self._populated_count += 1

    def merge(self, other: Sketch | None) -> None:
        """Merge another MinHash sketch with identical metadata.

        IncompatibleMergeError and CountOverflowError leave self unchanged.
        None is a no-op; a distinct source is never modified. Runtime failures,
        invalid private state, and concurrent use have no rollback guarantee.
        """

        if other is None:
            return
        if (
            self._profile != other._profile
            or self._length != other._length
            or self._domain != other._domain
            or self._algorithm != other._algorithm
        ):
            raise IncompatibleMergeError("metadata differs")
        if self._populated_count > MAX_UINT64 - other._populated_count:
            raise CountOverflowError("populated count")
        self._signature = [
            min(left, right)
            for left, right in zip(self._signature, other._signature, strict=True)
        ]
        self._populated_count += other._populated_count

    def jaccard_estimate(self, other: Sketch) -> float:
        """Return the MinHash Jaccard estimate against another sketch."""

        if (
            self._profile != other._profile
            or self._length != other._length
            or self._domain != other._domain
            or self._algorithm != other._algorithm
        ):
            raise IncompatibleMergeError("metadata differs")
        if self._populated_count == 0 and other._populated_count == 0:
            return 1.0
        if self._populated_count == 0 or other._populated_count == 0:
            return 0.0
        matches = sum(
            1
            for left, right in zip(self._signature, other._signature, strict=True)
            if left == right
        )
        return float(matches) / float(self._length)

    def populated_count(self) -> int:
        """Return the number of AddHash calls observed."""

        return self._populated_count

    def signature_length(self) -> int:
        """Return the configured signature length."""

        return self._length

    def signature(self) -> list[int]:
        """Return a copy of the signature words."""

        return list(self._signature)

    def signature_hex(self) -> list[str]:
        """Return signature words as fixed-width lowercase hex strings."""

        return [f"{value:016x}" for value in self._signature]

    def clone(self) -> Sketch:
        """Return a deep copy of the sketch."""

        clone = type(self)(self._profile, self._domain, self._algorithm)
        clone._signature = list(self._signature)
        clone._populated_count = self._populated_count
        return clone

    def marshal_binary(self) -> bytes:
        """Return deterministic protobuf bytes."""

        return _proto.serialize(self._to_proto())

    def _to_proto(self) -> Any:
        metadata = _proto.sketchpb.SketchMetadata(
            kind=_proto.SKETCH_KIND_MINHASH,
            wire_version=_proto.WIRE_VERSION,
            profile=self._profile,
            hash_domain=self._domain,
            hash_algo=_proto.HASH_ALGORITHM_HMAC_SHA256_64,
            representation_mode=_proto.REPRESENTATION_MINHASH_SIGNATURE,
        )
        metadata.minhash_signature_length = self._length

        body = _proto.sketchpb.MinHashSketch(
            signature=self._signature,
            populated_count=self._populated_count,
        )
        return _proto.sketchpb.Sketch(metadata=metadata, minhash=body)


def new(
    profile: str,
    domain: str = profiles.PROMPT_V1,
    algorithm: str = profiles.HMAC_SHA256_64,
) -> Sketch:
    """Construct an empty MinHash sketch."""

    return Sketch(profile, domain, algorithm)


def parse(data: bytes) -> Sketch:
    """Decode a MinHash sketch from deterministic protobuf bytes."""

    return Sketch.parse(data)
