# Whole-Period Distinct Users

Merging 610 daily summary files gives **52.00515815284467**, which rounds to
**52**, matching the exact union of released user labels. The two tools' exact
counts are 46 and 26; adding them gives 72 because 20 labels overlap.

This check uses llm-sketchkit 0.2.2, Python 3.11.14 and an Apple M4 Max on
October 10, 2026. The 305-day Claude Code and Codex records come from
[TraceLab v0.0.2](https://github.com/uw-syfi/TraceLab/releases/tag/v0.0.2), under
CC BY 4.0. Both tools use the released dataset's shared pseudonym namespace.
The estimate describes that namespace, with the same user field,
canonicalization, domain and public experiment key across summaries.

## Reproduce

Follow the pinned [summary-evidence setup and replay](https://github.com/llm-measurement/llm-measurement.github.io/blob/dddce13b327cbe9c175b4e80b8de04478aa15120/assets/summary-evidence/RUN.md)
through `features.py`. In that experiment directory, run:

```sh
venv/bin/python - <<'PY'
from pathlib import Path
import hashlib
import importlib.metadata
import json
from llm_sketchkit import hllpp, summary

dataset = Path('data/syfi_coding_trace.jsonl.gz')
with dataset.open('rb') as source:
    digest = hashlib.file_digest(source, 'sha256').hexdigest()
assert digest == '11ce51ec0a25e3d1d95b025bca2f7d1647e47571eb7cc968acd5fc64d4b4fb65'
states = {}
manifest = hashlib.sha256()
for tool in ('claude', 'codex'):
    files = sorted((Path('data/analyst/tracelab') / f'platform-{tool}').glob('*.json'))
    assert len(files) == 305
    merged = None
    for path in files:
        wire = path.read_bytes()
        manifest.update(f'{tool}/{path.name}:{hashlib.sha256(wire).hexdigest()}\n'.encode())
        current = hllpp.parse(summary.Envelope.parse(wire).sketches['distinct_users'].data)
        if merged is None:
            merged = current
        else:
            merged.merge(current)
    states[tool] = merged
combined = states['claude'].clone()
combined.merge(states['codex'])
result = {
    'sketchkit': importlib.metadata.version('llm-sketchkit'),
    'days': 305,
    'summary_files': 610,
    'dataset_sha256': digest,
    'summary_manifest_sha256': manifest.hexdigest(),
    'claude_estimate': states['claude'].estimate(),
    'codex_estimate': states['codex'].estimate(),
    'combined_estimate': combined.estimate(),
    'combined_rounded': round(combined.estimate()),
}
assert round(states['claude'].estimate()) == 46
assert round(states['codex'].estimate()) == 26
assert result['combined_rounded'] == 52
print(json.dumps(result, indent=2))
PY
```

## Recorded Output

```json
{
  "sketchkit": "0.2.2",
  "days": 305,
  "summary_files": 610,
  "dataset_sha256": "11ce51ec0a25e3d1d95b025bca2f7d1647e47571eb7cc968acd5fc64d4b4fb65",
  "summary_manifest_sha256": "bdd1727eff7fd7c8c90fac6ce6cbbf28814cf1b0a24efc9e5ea096750bc4c15a",
  "claude_estimate": 46.00403642189516,
  "codex_estimate": 26.00128945291158,
  "combined_estimate": 52.00515815284467,
  "combined_rounded": 52
}
```

The manifest hashes the relative filename and SHA-256 of each input summary,
ordered first by tool and then by day. No raw records or derived summaries are
included here. The original exact-label counts and source attribution remain in
the [summary evidence](https://llm-measurement.github.io/summaries.html).
