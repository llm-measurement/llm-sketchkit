# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay Erramilli and Codex
from contextlib import nullcontext

import pytest
from llm_sketchkit import bloom, frequentitems, hllpp, minhash, profiles


@pytest.mark.parametrize(
    "operation", ["add overflow", "merge overflow", "merge mismatch", "none merge"]
)
def test_bloom_mutation_failure_atomicity(operation: str) -> None:
    receiver = bloom.new("micro")
    source = bloom.new("micro")
    receiver.add_hash(1)
    source.add_hash(2)
    expected: type[ValueError] | None = None
    if operation in ("add overflow", "merge overflow"):
        # Exercise the uint64 boundary without iterating through 2**64 updates.
        receiver._inserted_count = (1 << 64) - 1
        expected = bloom.CountOverflowError
    elif operation == "merge mismatch":
        source = bloom.new("small")
        source.add_hash(2)
        expected = bloom.IncompatibleMergeError
    before = receiver.marshal_binary()
    source_before = source.marshal_binary()
    with pytest.raises(expected) if expected is not None else nullcontext():
        if operation == "add overflow":
            receiver.add_hash(3)
        else:
            receiver.merge(None if operation == "none merge" else source)
    assert receiver.marshal_binary() == before
    assert source.marshal_binary() == source_before


@pytest.mark.parametrize(
    "operation", ["add overflow", "merge overflow", "merge mismatch", "none merge"]
)
def test_minhash_mutation_failure_atomicity(operation: str) -> None:
    receiver = minhash.new("micro")
    source = minhash.new("micro")
    receiver.add_hash(1)
    source.add_hash(2)
    expected: type[ValueError] | None = None
    if operation in ("add overflow", "merge overflow"):
        # Exercise the uint64 boundary without iterating through 2**64 updates.
        receiver._populated_count = (1 << 64) - 1
        expected = minhash.CountOverflowError
    elif operation == "merge mismatch":
        source = minhash.new("small")
        source.add_hash(2)
        expected = minhash.IncompatibleMergeError
    before = receiver.marshal_binary()
    source_before = source.marshal_binary()
    with pytest.raises(expected) if expected is not None else nullcontext():
        if operation == "add overflow":
            receiver.add_hash(3)
        else:
            receiver.merge(None if operation == "none merge" else source)
    assert receiver.marshal_binary() == before
    assert source.marshal_binary() == source_before


@pytest.mark.parametrize("dense", [False, True])
@pytest.mark.parametrize(
    "operation", ["precision mismatch", "domain mismatch", "none merge"]
)
def test_hllpp_merge_failure_atomicity(operation: str, dense: bool) -> None:
    receiver = hllpp.new("micro")
    receiver.add_hash(1)
    if dense:
        receiver.force_dense()
    expected: type[ValueError] | None = None
    profile, domain = "micro", profiles.PROMPT_V1
    if operation == "precision mismatch":
        profile, expected = "small", hllpp.PrecisionMismatchError
    elif operation == "domain mismatch":
        domain, expected = profiles.USER_V1, hllpp.IncompatibleMergeError
    source = hllpp.new(profile, domain=domain)
    source.add_hash(2)
    before = receiver.marshal_binary()
    source_before = source.marshal_binary()
    with pytest.raises(expected) if expected is not None else nullcontext():
        receiver.merge(None if operation == "none merge" else source)
    assert receiver.marshal_binary() == before
    assert source.marshal_binary() == source_before


@pytest.mark.parametrize(
    "operation",
    [
        "negative weight",
        "add overflow",
        "merge overflow",
        "merge mismatch",
        "zero weight",
        "none merge",
    ],
)
def test_frequentitems_preflight_failure_atomicity(operation: str) -> None:
    receiver = frequentitems.new("micro")
    source = frequentitems.new("micro")
    source.add_hash(2, 1)
    weight = 1
    expected: type[ValueError] | None = None
    if operation == "negative weight":
        expected = frequentitems.NegativeWeightError
    elif operation in ("add overflow", "merge overflow"):
        weight, expected = (1 << 63) - 1, frequentitems.WeightOverflowError
    elif operation == "merge mismatch":
        source = frequentitems.new("small")
        source.add_hash(2, 1)
        expected = frequentitems.IncompatibleMergeError
    receiver.add_hash(1, weight)
    before = receiver.marshal_binary()
    source_before = source.marshal_binary()
    with pytest.raises(expected) if expected is not None else nullcontext():
        if operation == "negative weight":
            receiver.add_hash(3, -1)
        elif operation == "add overflow":
            receiver.add_hash(3, 1)
        elif operation == "zero weight":
            receiver.add_hash(3, 0)
        else:
            receiver.merge(None if operation == "none merge" else source)
    assert receiver.marshal_binary() == before
    assert source.marshal_binary() == source_before
