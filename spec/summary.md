# Summary Exchange v1

This optional JSON envelope carries existing sketch bytes and exact counters
between independently operated producers. It does not change `sketches.proto`.
It is a measurement contract, not an authentication or authorization protocol.

## Fields

Every field is required. Unknown fields and non-canonical encodings are rejected.

| Field | Meaning |
| --- | --- |
| `version` | Integer `1` |
| `producer_id` | Operator-assigned unique producer name |
| `epoch` | Unique process-start identifier, never reused |
| `sequence` | Positive, increasing snapshot number within that epoch |
| `scope_id` | Agreed measurement and identity-comparison scope |
| `accounting_id` | Version/fingerprint of the extraction and accounting rules |
| `key_id` | Operator-declared hashing key version; never the secret |
| `window_start_unix_nano` | Nonnegative, duration-aligned processing-time window start |
| `window_duration_unix_nano` | Positive duration, at most 24 hours |
| `observed_start_unix_nano` | Beginning of this epoch's observation within the window |
| `observed_end_unix_nano` | End of its observation, within the window |
| `emitted_at_unix_nano` | Snapshot creation time, at or after observed end |
| `counters` | Named nonnegative integer counters accumulated in this window/epoch |
| `sketches` | Named objects with `kind` and base64 `data` containing complete sketch state |

Identifiers and counter/sketch names use 1-128 ASCII letters, digits, `.`, `_`,
`:`, or `-`. Do not put sensitive identifiers in these cleartext fields.
Integers must fit a signed 64-bit integer; booleans are not integers. There are
at most 128 counters and 16 sketches per envelope. Supported kinds are `hllpp`,
`frequent_items`, `bloom`, and `minhash`. The complete encoded envelope is at most
8 MiB. JSON uses sorted object keys, no whitespace or trailing newline, and
standard padded base64. Sketch bytes must themselves round-trip canonically.

## Replacement, Restarts, and Combination

Only cumulative, per-window snapshots are supported. For each
`(producer_id, epoch, window)`, select the highest sequence. Identical repeated
snapshots are harmless; different content with the same identity/sequence is an
error. A replacement must preserve observation start and payload names, and
must not move observation end or counters backwards. Rebuild from the selected
snapshots rather than adding successive versions to an existing aggregate.

Different epochs from one producer contribute separately. Their observation
intervals must not overlap. A restart does not replace that producer's earlier
epoch. Gaps, late starts, and unfinished windows appear as partial coverage.
This policy applies to every kind, including Bloom and MinHash, whose complete
states contain additive update counters despite idempotent bitsets/signatures.

Combination requires identical scope, accounting, key identifier, window,
counter names, sketch names, and compatible sketch metadata. Comparison across
windows permits different window starts, but requires equal durations and the
same remaining measurement contract. These checks cannot prove that declared
keys, accounting rules, or producer identities are truthful.

The caller supplies an explicit, unique list of expected producers whose
underlying observation ownership is disjoint. Unlisted producers are rejected;
missing producers and incomplete observation intervals are reported. This is
not span deduplication: two collectors observing the same request will double
additive counters and frequent-items weights. Do not combine such inputs.
Distinct identities may legitimately occur in multiple disjoint request streams.

Input batches are limited to 1024 snapshots, 128 expected producers, and 64 MiB
of encoded data. Combination is local and deterministic in producer/epoch order.
It returns new state and never mutates supplied snapshots. Errors produce no
partial result. Sketch uncertainty remains the uncertainty of the underlying
method; cross-partition frequent-items bytes need not match a one-pass sketch.

## Trust and Privacy

Exchange files only through an authenticated, access-controlled route. Sender
identity and declarations must be checked against operator configuration, not
trusted because they occur in JSON. Matching key identifiers do not prove the
actual secrets match. Never combine identity summaries across tenants whose
policy forbids linkage. Use separate scopes and keys for those tenants.

Hash state is pseudonymous, not anonymous. Membership probing is possible for
anyone possessing the hashing secret. No secret or raw input is needed to merge
already compatible state. This format does not add signatures, encryption,
cross-tenant authorization, or exactly-once delivery.
