# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay Erramilli and Codex
"""Window-scoped exchange of existing sketches; see spec/summary.md."""

from __future__ import annotations

import base64
import binascii
import json
import re
from dataclasses import asdict, dataclass
from typing import Any

from google.protobuf.message import DecodeError  # type: ignore[import-untyped]

from . import _proto, bloom, frequentitems, hllpp, minhash

MAX_BYTES = 8 << 20
_MAX_INT = (1 << 63) - 1
_IDENTIFIER = re.compile(r"[A-Za-z0-9._:-]{1,128}", re.ASCII)


class SummaryError(ValueError):
    """Malformed, incompatible, conflicting, or oversized summary input."""


@dataclass
class Payload:
    data: bytes
    kind: str


@dataclass
class Envelope:
    accounting_id: str
    counters: dict[str, int]
    emitted_at_unix_nano: int
    epoch: str
    key_id: str
    observed_end_unix_nano: int
    observed_start_unix_nano: int
    producer_id: str
    scope_id: str
    sequence: int
    sketches: dict[str, Payload]
    version: int
    window_duration_unix_nano: int
    window_start_unix_nano: int

    def validate(self) -> None:
        for value in (
            self.version, self.sequence, self.emitted_at_unix_nano,
            self.observed_start_unix_nano, self.observed_end_unix_nano,
            self.window_duration_unix_nano, self.window_start_unix_nano,
        ):
            if type(value) is not int or not 0 <= value <= _MAX_INT:
                raise SummaryError("invalid summary integer")
        if self.version != 1 or self.sequence == 0:
            raise SummaryError("invalid summary version or sequence")
        for ident in (
            self.accounting_id, self.epoch, self.key_id,
            self.producer_id, self.scope_id,
        ):
            _check_identifier(ident)
        start = self.window_start_unix_nano
        duration = self.window_duration_unix_nano
        if (
            not 0 < duration <= 86_400_000_000_000
            or start + duration > _MAX_INT
            or start % duration != 0
            or not start <= self.observed_start_unix_nano
            <= self.observed_end_unix_nano <= start + duration
            or self.emitted_at_unix_nano < self.observed_end_unix_nano
        ):
            raise SummaryError("invalid summary observation interval")
        if (
            type(self.counters) is not dict or len(self.counters) > 128
            or type(self.sketches) is not dict or len(self.sketches) > 16
        ):
            raise SummaryError("invalid summary payload count")
        for name, count in self.counters.items():
            _check_identifier(name)
            if type(count) is not int or not 0 <= count <= _MAX_INT:
                raise SummaryError("invalid summary counter")
        size = 0
        for name, payload in self.sketches.items():
            _check_identifier(name)
            if not isinstance(payload, Payload) or type(payload.data) is not bytes:
                raise SummaryError("invalid summary sketch")
            size += len(payload.data)
            if size > MAX_BYTES or _merge_payload(payload).data != payload.data:
                raise SummaryError("invalid summary sketch state")

    def marshal_binary(self) -> bytes:
        """Return canonical JSON without mutating the receiver, including on error."""
        self.validate()
        document = asdict(self)
        document["sketches"] = {
            name: {"data": base64.b64encode(p.data).decode("ascii"), "kind": p.kind}
            for name, p in self.sketches.items()
        }
        data = json.dumps(document, sort_keys=True, separators=(",", ":")).encode()
        if len(data) > MAX_BYTES:
            raise SummaryError("summary exceeds size limit")
        return data

    @classmethod
    def parse(cls, data: bytes) -> Envelope:
        """Reject unknown fields, duplicates, noncanonical JSON, and invalid state."""
        if len(data) > MAX_BYTES:
            raise SummaryError("summary exceeds size limit")
        try:
            document: Any = json.loads(data)
            payloads = document["sketches"]
            if type(payloads) is not dict:
                raise SummaryError("invalid summary sketches")
            document["sketches"] = {
                name: Payload(
                    data=base64.b64decode(p["data"], validate=True), kind=p["kind"]
                )
                for name, p in payloads.items()
                if type(p) is dict and set(p) == {"data", "kind"}
            }
            if len(document["sketches"]) != len(payloads):
                raise SummaryError("invalid summary payload")
            envelope = cls(**document)
            if envelope.marshal_binary() != data:
                raise SummaryError("noncanonical summary JSON")
            return envelope
        except (ValueError, TypeError, KeyError, AttributeError, RecursionError) as exc:
            raise SummaryError("invalid summary JSON or state") from exc


def compatible(a: Envelope, b: Envelope) -> None:
    """Check comparison compatibility; window starts may differ. No mutation."""
    a.validate()
    b.validate()
    if (
        a.scope_id != b.scope_id or a.accounting_id != b.accounting_id
        or a.key_id != b.key_id
        or a.window_duration_unix_nano != b.window_duration_unix_nano
        or a.counters.keys() != b.counters.keys()
        or a.sketches.keys() != b.sketches.keys()
    ):
        raise SummaryError("incompatible summary measurement contract")
    for name, payload in a.sketches.items():
        other = b.sketches[name]
        left = _proto.parse_sketch(payload.data).metadata
        right = _proto.parse_sketch(other.data).metadata
        left.ClearField("representation_mode")
        right.ClearField("representation_mode")
        if payload.kind != other.kind or left != right:
            raise SummaryError("incompatible summary sketch metadata")


@dataclass
class Source:
    producer_id: str
    epoch: str
    sequence: int
    observed_start_unix_nano: int
    observed_end_unix_nano: int


@dataclass
class Result:
    counters: dict[str, int]
    sketches: dict[str, Payload]
    sources: list[Source]
    missing: list[str]
    partial: list[str]


def combine(inputs: list[Envelope], expected: list[str]) -> Result:
    """Rebuild one window from snapshots of declared disjoint input owners.

    All state returned is new. Errors do not mutate inputs or expose a partial
    result. Missing/partial producers are reported, not treated as measured zero.
    """
    if len(inputs) > 1024 or not 0 < len(expected) <= 128:
        raise SummaryError("invalid summary batch size")
    for name in expected:
        _check_identifier(name)
    if len(set(expected)) != len(expected):
        raise SummaryError("duplicate expected producer")
    encoded: list[tuple[Envelope, bytes]] = []
    size = 0
    for doc in inputs:
        data = doc.marshal_binary()
        size += len(data)
        if size > 64 << 20:
            raise SummaryError("summary batch exceeds size limit")
        if doc.producer_id not in expected:
            raise SummaryError("unexpected summary producer")
        encoded.append((doc, data))
    encoded.sort(key=lambda pair: (
        pair[0].producer_id, pair[0].epoch, pair[0].sequence
    ))
    selected: list[Envelope] = []
    previous_bytes = b""
    for doc, data in encoded:
        if selected:
            if doc.window_start_unix_nano != selected[0].window_start_unix_nano:
                raise SummaryError("cannot combine different windows")
            compatible(selected[0], doc)
            last = selected[-1]
            if (last.producer_id, last.epoch) == (doc.producer_id, doc.epoch):
                if last.sequence == doc.sequence:
                    if previous_bytes != data:
                        raise SummaryError("conflicting summary sequence")
                    continue
                if (
                    doc.observed_start_unix_nano != last.observed_start_unix_nano
                    or doc.observed_end_unix_nano < last.observed_end_unix_nano
                    or any(doc.counters[k] < v for k, v in last.counters.items())
                ):
                    raise SummaryError("summary observation or counter regressed")
                selected[-1] = doc
                previous_bytes = data
                continue
        selected.append(doc)
        previous_bytes = data
    result = Result({}, {}, [], [], [])
    intervals: dict[str, list[Envelope]] = {}
    for doc in selected:
        intervals.setdefault(doc.producer_id, []).append(doc)
        result.sources.append(Source(
            doc.producer_id, doc.epoch, doc.sequence,
            doc.observed_start_unix_nano, doc.observed_end_unix_nano,
        ))
        for name, value in doc.counters.items():
            total = result.counters.get(name, 0) + value
            if total > _MAX_INT:
                raise SummaryError("combined counter overflow")
            result.counters[name] = total
        for name, payload in doc.sketches.items():
            previous = result.sketches.get(name)
            result.sketches[name] = (
                _merge_payload(payload) if previous is None
                else _merge_payload(previous, payload)
            )
    for name in sorted(expected):
        if name not in intervals:
            result.missing.append(name)
            continue
        parts = sorted(intervals[name], key=lambda doc: doc.observed_start_unix_nano)
        end = parts[0].window_start_unix_nano
        partial = False
        for part in parts:
            if part.observed_start_unix_nano < end:
                raise SummaryError("overlapping producer epochs")
            partial = partial or part.observed_start_unix_nano != end
            end = part.observed_end_unix_nano
        if partial or end != (
            parts[0].window_start_unix_nano + parts[0].window_duration_unix_nano
        ):
            result.partial.append(name)
    return result


def _check_identifier(value: str) -> None:
    if not isinstance(value, str) or _IDENTIFIER.fullmatch(value) is None:
        raise SummaryError("invalid summary identifier")


def _merge_payload(a: Payload, b: Payload | None = None) -> Payload:
    if b is not None and a.kind != b.kind:
        raise SummaryError("sketch kinds differ")
    try:
        if a.kind == "hllpp":
            h = hllpp.Sketch.parse(a.data)
            if b is not None:
                h.merge(hllpp.Sketch.parse(b.data))
            data = h.marshal_binary()
        elif a.kind == "frequent_items":
            f = frequentitems.Sketch.parse(a.data)
            if b is not None:
                f.merge(frequentitems.Sketch.parse(b.data))
            data = f.marshal_binary()
        elif a.kind == "bloom":
            bl = bloom.Sketch.parse(a.data)
            if b is not None:
                bl.merge(bloom.Sketch.parse(b.data))
            data = bl.marshal_binary()
        elif a.kind == "minhash":
            m = minhash.Sketch.parse(a.data)
            if b is not None:
                m.merge(minhash.Sketch.parse(b.data))
            data = m.marshal_binary()
        else:
            raise SummaryError("unknown summary sketch kind")
        return Payload(data, a.kind)
    except (ValueError, binascii.Error, DecodeError) as exc:
        raise SummaryError("invalid or incompatible summary sketch state") from exc
