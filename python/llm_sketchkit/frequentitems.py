# Code authors: Vijay Erramilli and Codex
"""Pure-Python weighted frequent-items sketch implementation."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, cast

from . import _proto, profiles
from .hashfamily import MASK64

NO_FALSE_POSITIVES = "NO_FALSE_POSITIVES"
NO_FALSE_NEGATIVES = "NO_FALSE_NEGATIVES"
MAX_INT64 = (1 << 63) - 1


class FrequentItemsError(ValueError):
    """Base class for weighted frequent-items failures."""


class UnknownProfileError(FrequentItemsError):
    """Raised when a profile is not supported."""


class InvalidMapSizeError(FrequentItemsError):
    """Raised when wire metadata uses a non-profile map size."""


class NegativeWeightError(FrequentItemsError):
    """Raised when an update weight is negative."""


class WeightOverflowError(FrequentItemsError):
    """Raised when counters exceed signed int64 range."""


class IncompatibleMergeError(FrequentItemsError):
    """Raised when merge metadata differs."""


class InvalidQueryModeError(FrequentItemsError):
    """Raised when a query mode is not supported."""


class InvalidWireEncodingError(FrequentItemsError):
    """Raised when serialized bytes are malformed."""


@dataclass(frozen=True)
class Item:
    """A deterministic frequent-item interval."""

    hash: int
    estimate: int
    lower_bound: int
    upper_bound: int
    error: int


class Sketch:
    """Weighted mergeable frequent-items sketch over pre-hashed keys."""

    def __init__(
        self,
        profile: str,
        domain: str = profiles.PROMPT_V1,
        algorithm: str = profiles.HMAC_SHA256_64,
    ) -> None:
        map_size = profiles.FREQUENT_ITEMS_MAP_SIZES.get(profile)
        if map_size is None:
            raise UnknownProfileError(profile)
        if domain not in profiles.REGISTERED_DOMAINS:
            raise IncompatibleMergeError(domain)
        if algorithm != profiles.HMAC_SHA256_64:
            raise IncompatibleMergeError(algorithm)

        self._profile = profile
        self._map_size = map_size
        self._domain = domain
        self._algorithm = algorithm
        self._total_weight = 0
        self._max_error = 0
        self._items: dict[int, int] = {}

    @classmethod
    def parse(cls, data: bytes) -> Sketch:
        """Decode a weighted frequent-items sketch from protobuf bytes."""

        if len(data) > _proto.MAX_WIRE_BYTES:
            raise InvalidWireEncodingError("wire input too large")
        message = _proto.parse_sketch(data)
        metadata = message.metadata
        if not message.HasField("metadata") or not message.HasField("frequent_items"):
            raise InvalidWireEncodingError("missing frequent-items metadata or body")
        if metadata.kind != _proto.sketch_kind_frequent_items():
            raise InvalidWireEncodingError("wrong sketch kind")
        if metadata.wire_version != _proto.WIRE_VERSION:
            raise InvalidWireEncodingError("wrong wire version")
        if metadata.hash_algo != _proto.hash_algorithm_hmac_sha256_64():
            raise InvalidWireEncodingError("wrong hash algorithm")
        if (
            metadata.representation_mode
            != _proto.representation_frequent_items_bounded_map()
        ):
            raise InvalidWireEncodingError("wrong representation")

        profile = cast(str, metadata.profile)
        map_size = profiles.FREQUENT_ITEMS_MAP_SIZES.get(profile)
        if map_size is None:
            raise UnknownProfileError(profile)
        if cast(int, metadata.frequent_items_map_size) != map_size:
            raise InvalidMapSizeError(profile)

        sketch = cls(profile, cast(str, metadata.hash_domain), profiles.HMAC_SHA256_64)
        body = message.frequent_items
        total_weight = cast(int, body.total_weight)
        max_error = cast(int, body.max_error)
        if total_weight < 0 or max_error < 0 or max_error > total_weight:
            raise InvalidWireEncodingError("invalid total/error")
        if len(body.entries) > map_size:
            raise InvalidWireEncodingError("too many entries")

        sketch._total_weight = total_weight
        sketch._max_error = max_error
        previous = -1
        for entry in body.entries:
            key = cast(int, entry.hash)
            estimate = cast(int, entry.estimate)
            error = cast(int, entry.error)
            if key <= previous:
                raise InvalidWireEncodingError("entries unsorted")
            if estimate <= 0 or error != max_error or estimate <= max_error:
                raise InvalidWireEncodingError("invalid entry")
            sketch._items[key] = estimate
            previous = key
        return sketch

    def add_hash(self, value: int, weight: int) -> None:
        """Update a pre-hashed key by non-negative weight."""

        if weight < 0:
            raise NegativeWeightError(weight)
        if weight == 0:
            return
        if self._total_weight > MAX_INT64 - weight:
            raise WeightOverflowError("total weight")
        self._total_weight += weight
        self._add_residual(value & MASK64, weight)

    def merge(self, other: Sketch | None) -> None:
        """Merge another frequent-items sketch with identical metadata."""

        if other is None:
            return
        if (
            self._profile != other._profile
            or self._map_size != other._map_size
            or self._domain != other._domain
            or self._algorithm != other._algorithm
        ):
            raise IncompatibleMergeError("metadata differs")
        if self._total_weight > MAX_INT64 - other._total_weight:
            raise WeightOverflowError("total weight")
        if self._max_error > MAX_INT64 - other._max_error:
            raise WeightOverflowError("max error")

        combined: dict[int, int] = {}
        for key, estimate in self._items.items():
            _add_combined_residual(combined, key, estimate - self._max_error)
        for key, estimate in other._items.items():
            _add_combined_residual(combined, key, estimate - other._max_error)

        entries = sorted(
            ((key, residual) for key, residual in combined.items() if residual > 0),
            key=lambda entry: entry[0],
        )
        total_weight = self._total_weight + other._total_weight
        max_error = self._max_error + other._max_error
        self._reset()
        self._total_weight = total_weight
        self._max_error = max_error
        for key, residual in entries:
            self._add_residual(key, residual)

    def estimate_hash(self, value: int) -> int:
        """Return the current upper estimate for a tracked key, or zero."""

        return self._items.get(value & MASK64, 0)

    def lower_bound_hash(self, value: int) -> int:
        """Return the deterministic lower bound for a tracked key."""

        estimate = self._items.get(value & MASK64)
        if estimate is None:
            return 0
        return max(0, estimate - self._max_error)

    def upper_bound_hash(self, value: int) -> int:
        """Return the deterministic upper bound for a key."""

        return self._items.get(value & MASK64, self._max_error)

    def frequent_items(self, mode: str) -> list[Item]:
        """Return deterministic frequent items under the selected query mode."""

        out: list[Item] = []
        for item in self.items():
            if mode == NO_FALSE_NEGATIVES:
                if item.upper_bound > self._max_error:
                    out.append(item)
            elif mode == NO_FALSE_POSITIVES:
                if item.lower_bound > self._max_error:
                    out.append(item)
            else:
                raise InvalidQueryModeError(mode)
        return sorted(out, key=lambda item: (-item.estimate, item.hash))

    def items(self) -> list[Item]:
        """Return tracked items sorted by hash."""

        return [
            self._item(key, estimate)
            for key, estimate in sorted(self._items.items(), key=lambda entry: entry[0])
        ]

    def total_weight(self) -> int:
        """Return total non-negative update weight observed."""

        return self._total_weight

    def max_error(self) -> int:
        """Return the global deterministic error bound."""

        return self._max_error

    def map_size(self) -> int:
        """Return the configured tracked-item bound."""

        return self._map_size

    def __len__(self) -> int:
        return len(self._items)

    def clone(self) -> Sketch:
        """Return a deep copy of the sketch."""

        clone = type(self)(self._profile, self._domain, self._algorithm)
        clone._total_weight = self._total_weight
        clone._max_error = self._max_error
        clone._items = dict(self._items)
        return clone

    def marshal_binary(self) -> bytes:
        """Return deterministic protobuf bytes."""

        return _proto.serialize(self._to_proto())

    def _add_residual(self, key: int, residual: int) -> None:
        if residual <= 0:
            return
        current = self._items.get(key)
        if current is not None:
            if current > MAX_INT64 - residual:
                raise WeightOverflowError("counter")
            self._items[key] = current + residual
            return
        if len(self._items) < self._map_size:
            if self._max_error > MAX_INT64 - residual:
                raise WeightOverflowError("counter")
            self._items[key] = self._max_error + residual
            return

        consumed = self._prune_for_residual(residual)
        remaining = residual - consumed
        if remaining > 0:
            if len(self._items) >= self._map_size:
                raise InvalidMapSizeError("no free counter")
            if self._max_error > MAX_INT64 - remaining:
                raise WeightOverflowError("counter")
            self._items[key] = self._max_error + remaining

    def _prune_for_residual(self, residual: int) -> int:
        if not self._items:
            return 0
        min_key, min_estimate = min(
            self._items.items(), key=lambda entry: (entry[1], entry[0])
        )
        min_residual = min_estimate - self._max_error
        if residual < min_residual:
            if self._max_error > MAX_INT64 - residual:
                raise WeightOverflowError("max error")
            self._max_error += residual
            return residual

        if self._max_error > MAX_INT64 - min_residual:
            raise WeightOverflowError("max error")
        self._max_error += min_residual
        self._remove_expired()
        return min_residual

    def _remove_expired(self) -> None:
        expired = [
            key for key, estimate in self._items.items() if estimate <= self._max_error
        ]
        for key in expired:
            del self._items[key]

    def _reset(self) -> None:
        self._items.clear()
        self._total_weight = 0
        self._max_error = 0

    def _item(self, key: int, estimate: int) -> Item:
        lower = max(0, estimate - self._max_error)
        return Item(
            hash=key,
            estimate=estimate,
            lower_bound=lower,
            upper_bound=estimate,
            error=estimate - lower,
        )

    def _to_proto(self) -> Any:
        metadata = _proto.sketchpb.SketchMetadata(
            kind=_proto.sketch_kind_frequent_items(),
            wire_version=_proto.WIRE_VERSION,
            profile=self._profile,
            hash_domain=self._domain,
            hash_algo=_proto.hash_algorithm_hmac_sha256_64(),
            representation_mode=_proto.representation_frequent_items_bounded_map(),
        )
        metadata.frequent_items_map_size = self._map_size

        body = _proto.sketchpb.FrequentItemsSketch(
            total_weight=self._total_weight,
            max_error=self._max_error,
        )
        for item in self.items():
            body.entries.append(
                _proto.sketchpb.FrequentItemsEntry(
                    hash=item.hash,
                    estimate=item.estimate,
                    error=item.error,
                )
            )
        return _proto.sketchpb.Sketch(metadata=metadata, frequent_items=body)


def new(
    profile: str,
    domain: str = profiles.PROMPT_V1,
    algorithm: str = profiles.HMAC_SHA256_64,
) -> Sketch:
    """Construct an empty weighted frequent-items sketch."""

    return Sketch(profile, domain, algorithm)


def parse(data: bytes) -> Sketch:
    """Decode a weighted frequent-items sketch from protobuf bytes."""

    return Sketch.parse(data)


def _add_combined_residual(combined: dict[int, int], key: int, residual: int) -> None:
    if residual <= 0:
        return
    current = combined.get(key, 0)
    combined[key] = min(MAX_INT64, current + residual)
