# Mutation And Error Reference

Go method comments and Python docstrings state each method's failure behavior.
This reference collects those contracts for integrators. The
[operational contracts](OPERATIONS.md) also apply.

## What Unchanged Means

An unchanged receiver has the same counters, registers, representation, and
serialized bytes as before the call. This is failure atomicity, not thread safety:
all access to one instance still needs external synchronization.

The guarantees below cover the named Go error returns and Python exceptions, using
documented argument types and sketch state. They do not cover Go panics, Python
runtime failures such as `MemoryError`, direct changes to private fields, or
concurrent access. Error-message text is not a stable way to determine whether
rollback occurred.

## Mutating Methods

| Sketch | Go / Python method | Receiver on failure |
|---|---|---|
| HLL++ | `AddHash` / `add_hash` | No recoverable Go error return or documented Python error for valid state and integer input. Runtime failures have no rollback guarantee. |
| HLL++ | `ForceDense` / `force_dense` | No recoverable Go error return or documented Python error for valid state. Runtime failures have no rollback guarantee. |
| HLL++ | `Merge` / `merge` | Precision and metadata mismatch errors leave the receiver unchanged. |
| Bloom | `AddHash` / `add_hash` | Count overflow leaves the receiver unchanged, including the bitset. |
| Bloom | `Merge` / `merge` | Metadata mismatch and count overflow leave the receiver unchanged. |
| MinHash | `AddHash` / `add_hash` | Count overflow leaves the receiver unchanged, including the signature. |
| MinHash | `Merge` / `merge` | Metadata mismatch and count overflow leave the receiver unchanged. |
| Frequent-items | `AddHash` / `add_hash` | Negative weight and rejection by the initial total-weight overflow check leave the receiver unchanged. Later counter/pruning errors can leave weight or counters changed. |
| Frequent-items | `Merge` / `merge` | Metadata mismatch and rejection by the initial total-weight or max-error overflow checks leave the receiver unchanged. Errors during counter rebuilding can leave a partially rebuilt receiver. |

All merges accept `nil` / `None` as a successful no-op. A distinct source sketch is
not modified, whether the merge succeeds or fails. In a self-merge, source and
receiver are the same object; there is no separate source snapshot.

Frequent-items weight zero is a successful no-op. Its `ErrWeightOverflow` /
`WeightOverflowError` can describe either a preflight rejection or a later failure;
the error class alone is not an unchanged-state promise. Discard a failed receiver
unless the documented unchanged case is established. Use a clone when preserving
the original is required.

## Applying A Change Transactionally

Work on a clone and replace the application-owned reference only after success.
Keep the same external lock across cloning, mutation, and replacement when sharing
an instance.

```go
candidate := current.Clone()
if err := candidate.Merge(incoming); err != nil {
    return err // current is unchanged; discard candidate.
}
current = candidate
```

```python
candidate = current.clone()
candidate.merge(incoming)  # On an exception, discard candidate.
current = candidate
```

## Methods Without Receiver Mutation

Constructors and `Parse` / `parse` return a new instance rather than loading into
an existing receiver. A failed parse does not replace an application-owned sketch.
`Clone` / `clone` creates independent state. Queries and serialization do not mutate
the receiver, including when a query or serialization reports an error. They still
must not run concurrently with a mutation on the same instance.

## Summary Exchange

The optional `summary` package wraps existing sketch bytes without changing them.
`Envelope.Validate`, `MarshalBinary`, `Parse`, `Compatible`, and `Combine` (Python:
`validate`, `marshal_binary`, `parse`, `compatible`, `combine`) never mutate supplied
state. Combination returns independent counters and sketch bytes. Any reported
error returns no partial aggregate. Concurrent mutation of inputs is unsupported.

See the [exchange example](../examples/summary-exchange/README.md) and
[format contract](../spec/summary.md). This API is available in the source checkout;
it is not part of the `0.1.0` package release.
