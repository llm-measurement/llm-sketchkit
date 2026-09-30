# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay and Codex
"""Generate two synthetic, disjoint producers for the summary-exchange example."""

import argparse
from pathlib import Path

from llm_sketchkit import (
    USER_V1,
    canonicalize_text_v1,
    frequentitems,
    hash64,
    hllpp,
    secret_from_env,
    summary,
)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out", type=Path, default=Path(__file__).parent / "generated")
    args = parser.parse_args()
    secret = secret_from_env("LLM_SKETCHKIT_SECRET")
    batches = {
        "platform": [("support", 1240), ("research", 8900)],
        "data": [("support", 980), ("coding", 3600)],
    }
    args.out.mkdir(parents=True, exist_ok=True)
    for producer, events in batches.items():
        users = hllpp.new("small", USER_V1)
        tokens = frequentitems.Sketch("small", USER_V1)
        for key, weight in events:
            digest = hash64(secret, USER_V1, canonicalize_text_v1(key))
            users.add_hash(digest)
            tokens.add_hash(digest, weight)
        envelope = summary.Envelope(
            accounting_id="reported-tokens-v1",
            counters={"requests": len(events), "tokens": sum(w for _, w in events)},
            emitted_at_unix_nano=180_000_000_000,
            epoch="synthetic-start-1",
            key_id="synthetic-key-v1",
            observed_end_unix_nano=180_000_000_000,
            observed_start_unix_nano=120_000_000_000,
            producer_id=producer,
            scope_id="synthetic-gateway",
            sequence=1,
            sketches={
                "users": summary.Payload(users.marshal_binary(), "hllpp"),
                "tokens": summary.Payload(tokens.marshal_binary(), "frequent_items"),
            },
            version=1,
            window_duration_unix_nano=60_000_000_000,
            window_start_unix_nano=120_000_000_000,
        )
        (args.out / f"{producer}.json").write_bytes(envelope.marshal_binary())
    print("wrote platform.json and data.json")


if __name__ == "__main__":
    main()
