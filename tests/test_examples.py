# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay and Codex
"""Keep copyable README recipes and the summary-exchange walkthrough runnable."""

import json
import os
import re
import subprocess
import sys
from pathlib import Path

import pytest
from llm_sketchkit import bloom, frequentitems, hash, hllpp, minhash

ROOT = Path(__file__).resolve().parents[1]
SECRET = "test-only-not-a-deployment-secret-0123456789"


def test_readme_python_recipes() -> None:
    readme = (ROOT / "README.md").read_text()
    distinct = re.search(r"python - <<'PY'\n(.*?)\nPY", readme, re.S)
    assert distinct is not None
    snippets = [distinct.group(1), *re.findall(r"```python\n(.*?)```", readme, re.S)]
    expected = [
        "estimated distinct prompts: 1\n",
        "research/synthesis estimate=8900 bounds=[8900, 8900]\n"
        "coding/review estimate=3600 bounds=[3600, 3600]\n"
        "support/refund estimate=2220 bounds=[2220, 2220]\n",
        "merged distinct prompts: 2\n",
    ]
    assert len(snippets) == len(expected)
    for snippet, output in zip(snippets, expected, strict=True):
        result = subprocess.run(
            [sys.executable, "-c", snippet], check=True, capture_output=True,
            text=True, env={**os.environ, "LLM_SKETCHKIT_SECRET": SECRET},
        )
        assert result.stdout == output
        assert output.strip() in readme


def test_readme_links_are_absolute() -> None:
    readme = (ROOT / "README.md").read_text()
    links = re.findall(r"\]\(([^)]+)\)", readme)
    assert links
    assert all(link.startswith("https://") for link in links)


def test_summary_exchange(tmp_path: Path) -> None:
    example = ROOT / "examples" / "summary-exchange"
    subprocess.run(
        [sys.executable, str(example / "generate.py"), "--out", str(tmp_path)],
        check=True, capture_output=True,
        env={**os.environ, "LLM_SKETCHKIT_SECRET": SECRET},
    )
    files = [str(tmp_path / f"{name}.json") for name in ("platform", "data")]
    env = {k: v for k, v in os.environ.items() if k != "LLM_SKETCHKIT_SECRET"}
    command = [
        sys.executable, str(example / "combine.py"),
        "--expected", "platform", "data", "--window-start", "120000000000", "--",
    ]
    result = subprocess.run(
        [*command, *files, files[0]], check=True, capture_output=True,
        text=True, env=env,
    )
    report = json.loads(result.stdout)
    assert report["counters"] == {"requests": 4, "tokens": 14720}
    assert round(report["distinct_estimates"]["users"]) == 3
    assert report["missing"] == report["partial"] == []
    assert len(report["sources"]) == 2
    items = report["heavy_items"]["tokens"]
    assert [item["estimate"] for item in items] == [8900, 3600, 2220]
    assert all(i["lower_bound"] == i["upper_bound"] == i["estimate"] for i in items)
    for file in files:
        raw = Path(file).read_text()
        assert SECRET not in raw
        assert all(name not in raw for name in ("support", "research", "coding"))
    missing = subprocess.run(
        [*command, files[0]], check=True, capture_output=True, text=True, env=env,
    )
    assert json.loads(missing.stdout)["missing"] == ["data"]


def test_unregistered_hash_domain_message() -> None:
    secret = hash.Secret(SECRET.encode())
    for domain in ("unknown:v1", "model:v1", "api-key:v1", "tenant:v1"):
        with pytest.raises(hash.UnregisteredDomainError) as caught:
            hash.hash64(secret, domain, b"value")
        assert str(caught.value) == f"unregistered hash domain: {domain}"
        for module in (bloom, frequentitems, hllpp, minhash):
            with pytest.raises(module.IncompatibleMergeError) as constructor_error:
                module.Sketch("small", domain)
            assert str(constructor_error.value) == f"unregistered hash domain: {domain}"


def test_frequent_items_order() -> None:
    sketch = frequentitems.Sketch("small")
    for digest, weight in ((3, 20), (2, 30), (1, 20)):
        sketch.add_hash(digest, weight)
    for mode in (frequentitems.NO_FALSE_NEGATIVES, frequentitems.NO_FALSE_POSITIVES):
        assert [i.hash for i in sketch.frequent_items(mode)] == [2, 1, 3]
