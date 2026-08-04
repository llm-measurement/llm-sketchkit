# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay Erramilli and Codex
"""Pure-Python HLL++ sketch implementation."""

from __future__ import annotations

import math
from collections.abc import Iterable
from typing import Any, cast

from . import _proto, profiles
from .hashfamily import MASK64
from .hllpp_bias import BIAS_DATA, RAW_ESTIMATE_DATA


class HLLPPError(ValueError):
    """Base class for HLL++ failures."""


class UnknownProfileError(HLLPPError):
    """Raised when a profile is not supported."""


class PrecisionMismatchError(HLLPPError):
    """Raised when HLL++ precision metadata does not match."""


class IncompatibleMergeError(HLLPPError):
    """Raised when merge metadata differs."""


class InvalidWireEncodingError(HLLPPError):
    """Raised when serialized bytes are malformed."""


class Sketch:
    """Mergeable HLL++ sketch over pre-hashed unsigned 64-bit values."""

    def __init__(
        self,
        profile: str,
        domain: str = profiles.PROMPT_V1,
        algorithm: str = profiles.HMAC_SHA256_64,
    ) -> None:
        config = profiles.HLLPP_PROFILES.get(profile)
        if config is None:
            raise UnknownProfileError(profile)
        if domain not in profiles.REGISTERED_DOMAINS:
            raise IncompatibleMergeError(domain)
        if algorithm != profiles.HMAC_SHA256_64:
            raise IncompatibleMergeError(algorithm)

        self._profile = profile
        self._p = config.normal_precision
        self._sp = config.sparse_precision
        self._promotion_threshold = config.promotion_threshold
        self._domain = domain
        self._algorithm = algorithm
        self._sparse: dict[int, int] | None = {}
        self._dense: list[int] | None = None

    @classmethod
    def parse(cls, data: bytes) -> Sketch:
        """Decode a HLL++ sketch from deterministic protobuf bytes."""

        if len(data) > _proto.MAX_WIRE_BYTES:
            raise InvalidWireEncodingError("wire input too large")
        message = _proto.parse_sketch(data)
        metadata = message.metadata
        if not message.HasField("metadata") or not message.HasField("hllpp"):
            raise InvalidWireEncodingError("missing HLL++ metadata or body")
        if metadata.kind != _proto.sketch_kind_hllpp():
            raise InvalidWireEncodingError("wrong sketch kind")
        if metadata.wire_version != _proto.WIRE_VERSION:
            raise InvalidWireEncodingError("wrong wire version")
        if metadata.hash_algo != _proto.hash_algorithm_hmac_sha256_64():
            raise InvalidWireEncodingError("wrong hash algorithm")

        profile = cast(str, metadata.profile)
        config = profiles.HLLPP_PROFILES.get(profile)
        if config is None:
            raise UnknownProfileError(profile)
        normal_precision = cast(int, metadata.hllpp_normal_precision)
        sparse_precision = cast(int, metadata.hllpp_sparse_precision)
        if (
            normal_precision != config.normal_precision
            or sparse_precision != config.sparse_precision
        ):
            raise PrecisionMismatchError(profile)

        sketch = cls(profile, cast(str, metadata.hash_domain), profiles.HMAC_SHA256_64)
        body = message.hllpp
        mode = cast(int, metadata.representation_mode)
        if mode == _proto.representation_hllpp_sparse():
            if len(body.sparse_registers) > config.promotion_threshold:
                raise InvalidWireEncodingError("too many sparse registers")
            sketch._sparse = {}
            sketch._dense = None
            previous = -1
            max_sparse_index = 1 << sparse_precision
            for register in body.sparse_registers:
                index = cast(int, register.index)
                value = cast(int, register.value)
                if index <= previous:
                    raise InvalidWireEncodingError("sparse registers unsorted")
                if index >= max_sparse_index:
                    raise InvalidWireEncodingError("sparse index out of range")
                if value == 0 or value > 64 - sparse_precision + 1:
                    raise InvalidWireEncodingError("invalid sparse rank")
                sketch._sparse[index] = value
                previous = index
            if len(cast(bytes, body.dense_registers)) != 0:
                raise InvalidWireEncodingError("sparse body contains dense bytes")
        elif mode == _proto.representation_hllpp_dense():
            dense_registers = cast(bytes, body.dense_registers)
            expected = 1 << normal_precision
            if len(dense_registers) != expected:
                raise InvalidWireEncodingError("wrong dense register length")
            if len(body.sparse_registers) != 0:
                raise InvalidWireEncodingError("dense body contains sparse registers")
            max_dense_rank = 64 - normal_precision + 1
            if any(rank > max_dense_rank for rank in dense_registers):
                raise InvalidWireEncodingError("invalid dense rank")
            sketch._sparse = None
            sketch._dense = list(dense_registers)
        else:
            raise InvalidWireEncodingError("wrong representation")

        return sketch

    def add_hash(self, value: int) -> None:
        """Update the sketch with a pre-hashed unsigned 64-bit value."""

        value &= MASK64
        if self._sparse is not None:
            index, rank = _sparse_register(value, self._sp)
            current = self._sparse.get(index)
            if current is None or rank > current:
                self._sparse[index] = rank
            if len(self._sparse) > self._promotion_threshold:
                self.force_dense()
            return

        if self._dense is None:
            raise InvalidWireEncodingError("missing dense registers")
        index, rank = _dense_register(value, self._p)
        if rank > self._dense[index]:
            self._dense[index] = rank

    def merge(self, other: Sketch | None) -> None:
        """Merge another HLL++ sketch with identical metadata."""

        if other is None:
            return
        if self._p != other._p or self._sp != other._sp:
            raise PrecisionMismatchError("precision mismatch")
        if (
            self._profile != other._profile
            or self._domain != other._domain
            or self._algorithm != other._algorithm
        ):
            raise IncompatibleMergeError("metadata differs")

        if self._sparse is not None and other._sparse is not None:
            for index, rank in other._sparse.items():
                current = self._sparse.get(index)
                if current is None or rank > current:
                    self._sparse[index] = rank
            if len(self._sparse) > self._promotion_threshold:
                self.force_dense()
        elif self._sparse is not None:
            self.force_dense()
            _merge_dense(self._dense_checked(), other._dense_checked())
        elif other._sparse is not None:
            dense = self._dense_checked()
            for index, rank in other._sparse.items():
                dense_index, dense_rank = _sparse_to_dense_register(
                    index, rank, self._p, self._sp
                )
                if dense_rank > dense[dense_index]:
                    dense[dense_index] = dense_rank
        else:
            _merge_dense(self._dense_checked(), other._dense_checked())

    def estimate(self) -> float:
        """Return the HLL++ cardinality estimate."""

        if self._sparse is not None:
            return _linear_counting(1 << self._sp, len(self._sparse))
        return self._dense_estimate()

    def representation_mode(self) -> str:
        """Return the active representation name."""

        if self._sparse is not None:
            return "HLLPP_SPARSE"
        return "HLLPP_DENSE"

    def sparse_count(self) -> int:
        """Return the number of sparse registers currently stored."""

        if self._sparse is None:
            return 0
        return len(self._sparse)

    def dense_nonzero_count(self) -> int:
        """Return the number of non-zero dense registers."""

        if self._dense is None:
            return 0
        return sum(1 for rank in self._dense if rank != 0)

    def sparse_registers(self) -> list[tuple[int, int]]:
        """Return sparse registers sorted by index."""

        if self._sparse is None:
            return []
        return sorted(self._sparse.items())

    def dense_registers(self) -> bytes:
        """Return dense registers as canonical bytes, or empty bytes in sparse mode."""

        if self._dense is None:
            return b""
        return bytes(self._dense)

    def force_dense(self) -> None:
        """Promote a sparse sketch to dense representation."""

        if self._sparse is None:
            return
        dense = [0] * (1 << self._p)
        for index, rank in self._sparse.items():
            dense_index, dense_rank = _sparse_to_dense_register(
                index, rank, self._p, self._sp
            )
            if dense_rank > dense[dense_index]:
                dense[dense_index] = dense_rank
        self._sparse = None
        self._dense = dense

    def clone(self) -> Sketch:
        """Return a deep copy of the sketch."""

        clone = type(self)(self._profile, self._domain, self._algorithm)
        if self._sparse is not None:
            clone._sparse = dict(self._sparse)
            clone._dense = None
        else:
            clone._sparse = None
            clone._dense = list(self._dense_checked())
        return clone

    def marshal_binary(self) -> bytes:
        """Return deterministic protobuf bytes."""

        return _proto.serialize(self._to_proto())

    def _dense_checked(self) -> list[int]:
        if self._dense is None:
            raise InvalidWireEncodingError("missing dense registers")
        return self._dense

    def _dense_estimate(self) -> float:
        dense = self._dense_checked()
        m = 1 << self._p
        total = 0.0
        zero_count = 0
        for rank in dense:
            total += math.ldexp(1.0, -rank)
            if rank == 0:
                zero_count += 1

        raw = _alpha(m) * float(m * m) / total
        estimate_prime = raw
        if raw <= 5.0 * float(m):
            estimate_prime = raw - _estimate_bias(raw, self._p)

        if zero_count != 0:
            lc = _linear_counting(m, m - zero_count)
            if lc <= _linear_counting_threshold(self._p):
                return lc
        return estimate_prime

    def _to_proto(self) -> Any:
        metadata = _proto.sketchpb.SketchMetadata(
            kind=_proto.sketch_kind_hllpp(),
            wire_version=_proto.WIRE_VERSION,
            profile=self._profile,
            hash_domain=self._domain,
            hash_algo=_proto.hash_algorithm_hmac_sha256_64(),
            representation_mode=(
                _proto.representation_hllpp_sparse()
                if self._sparse is not None
                else _proto.representation_hllpp_dense()
            ),
        )
        metadata.hllpp_normal_precision = self._p
        metadata.hllpp_sparse_precision = self._sp

        body = _proto.sketchpb.HllppSketch()
        if self._sparse is not None:
            for index, rank in sorted(self._sparse.items()):
                body.sparse_registers.append(
                    _proto.sketchpb.HllppSparseRegister(index=index, value=rank)
                )
        else:
            body.dense_registers = bytes(self._dense_checked())

        return _proto.sketchpb.Sketch(metadata=metadata, hllpp=body)


def new(
    profile: str,
    domain: str = profiles.PROMPT_V1,
    algorithm: str = profiles.HMAC_SHA256_64,
) -> Sketch:
    """Construct an empty HLL++ sketch."""

    return Sketch(profile, domain, algorithm)


def parse(data: bytes) -> Sketch:
    """Decode a HLL++ sketch from deterministic protobuf bytes."""

    return Sketch.parse(data)


def _merge_dense(dst: list[int], src: list[int]) -> None:
    for i, rank in enumerate(src):
        if rank > dst[i]:
            dst[i] = rank


def _dense_register(value: int, precision: int) -> tuple[int, int]:
    index = value >> (64 - precision)
    rank = _register_rank((value << precision) & MASK64, 64 - precision)
    return index, rank


def _sparse_register(value: int, sparse_precision: int) -> tuple[int, int]:
    index = value >> (64 - sparse_precision)
    rank = _register_rank((value << sparse_precision) & MASK64, 64 - sparse_precision)
    return index, rank


def _sparse_to_dense_register(
    sparse_index: int, sparse_rank: int, precision: int, sparse_precision: int
) -> tuple[int, int]:
    extra_bits = sparse_precision - precision
    dense_index = sparse_index >> extra_bits
    extra = sparse_index & ((1 << extra_bits) - 1)
    if extra != 0:
        shifted = (extra << (64 - extra_bits)) & MASK64
        return dense_index, _register_rank(shifted, extra_bits)
    return dense_index, extra_bits + sparse_rank


def _register_rank(shifted: int, width: int) -> int:
    leading = 64 if shifted == 0 else 64 - shifted.bit_length()
    return min(leading + 1, width + 1)


def _linear_counting(register_count: int, occupied: int) -> float:
    if occupied == 0:
        return 0.0
    if occupied >= register_count:
        return float(register_count)
    return float(register_count) * math.log(
        float(register_count) / float(register_count - occupied)
    )


def _alpha(m: int) -> float:
    if m == 16:
        return 0.673
    if m == 32:
        return 0.697
    if m == 64:
        return 0.709
    return 0.7213 / (1.0 + 1.079 / float(m))


def _linear_counting_threshold(precision: int) -> float:
    thresholds = {
        4: 10.0,
        5: 20.0,
        6: 40.0,
        7: 80.0,
        8: 220.0,
        9: 400.0,
        10: 900.0,
        11: 1800.0,
        12: 3100.0,
        13: 6500.0,
        14: 11500.0,
        15: 20000.0,
        16: 50000.0,
        17: 120000.0,
        18: 350000.0,
    }
    return thresholds.get(precision, 5.0 * float(1 << precision) / 2.0)


def _estimate_bias(estimate: float, precision: int) -> float:
    estimates = RAW_ESTIMATE_DATA.get(precision, ())
    biases = BIAS_DATA.get(precision, ())
    if not estimates or len(estimates) != len(biases):
        return 0.0

    indexes = sorted(
        range(len(estimates)), key=lambda index: (estimate - estimates[index]) ** 2
    )
    neighbors = min(6, len(indexes))
    return sum(biases[index] for index in indexes[:neighbors]) / float(neighbors)


def _add_all(sketch: Sketch, values: Iterable[int]) -> Sketch:
    for value in values:
        sketch.add_hash(value)
    return sketch
