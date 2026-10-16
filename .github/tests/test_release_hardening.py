# SPDX-License-Identifier: Apache-2.0
"""Exercise the release workflow's source guard and artifact isolation."""

from __future__ import annotations

import os
import re
import subprocess
from pathlib import Path

import pytest
import yaml

ROOT = Path(__file__).resolve().parents[2]


@pytest.mark.parametrize(
    ("tag", "accepted"),
    [
        ("v0.2.2", True), ("v0.3.0-alpha.1", True),
        ("main", False), ("ca5d39b", False), ("refs/tags/v0.2.2", False),
        ("vbranch", False), ("vmissing", False), ("vnotcommit", False),
        ("v0.2.2^{commit}", False), ("v0.2.2\ncommit=forged", False),
        ("v0.2.2; exit 0", False), ("", False),
    ],
)
def test_release_requires_existing_version_tag(
    tmp_path: Path, tag: str, accepted: bool,
) -> None:
    workflow = yaml.safe_load((ROOT / ".github/workflows/release.yml").read_text())
    steps = workflow["jobs"]["build"]["steps"]
    index, resolve = next(
        (i, step) for i, step in enumerate(steps) if step.get("id") == "tag"
    )
    assert steps[0]["with"]["fetch-depth"] == 0
    assert steps[0]["with"]["ref"] == "${{ github.workflow_sha }}"
    assert index < next(
        i for i, step in enumerate(steps) if "pip install" in step.get("run", "")
    )
    assert "${{" not in resolve["run"]
    assert resolve["env"]["RELEASE_TAG"] == (
        "${{ github.event.release.tag_name || inputs.release_tag }}"
    )
    source = next(s for s in steps if s.get("name") == "Check out release source")
    assert source["with"]["ref"] == "${{ steps.tag.outputs.commit }}"
    # Git checks actual ref syntax; read-only lookups model fetched remote tags.
    lookups = r'''
git() {
  case "$1" in
    check-ref-format) command git "$@" ;;
    show-ref)
      [[ "$2" == --verify && "$3" == --quiet ]] || return 97
      case "$4" in
        refs/tags/v0.2.2|refs/tags/v0.3.0-alpha.1|refs/tags/vnotcommit) return 0 ;;
        *) return 1 ;;
      esac ;;
    rev-parse)
      [[ "$2" == --verify ]] || return 97
      case "$3" in
        'refs/tags/v0.2.2^{commit}'|'refs/tags/v0.3.0-alpha.1^{commit}')
          printf '%040d\n' 1 ;;
        *) return 1 ;;
      esac ;;
    *) return 97 ;;
  esac
}
'''
    output = tmp_path / "output"
    result = subprocess.run(
        ["bash", "--noprofile", "--norc", "-eo", "pipefail", "-c",
         lookups + resolve["run"]],
        env={**os.environ, "RELEASE_TAG": tag, "GITHUB_OUTPUT": str(output)},
        capture_output=True, text=True, check=False,
    )
    assert (result.returncode == 0) is accepted
    if accepted:
        assert output.read_text() == f"commit={1:040d}\n"
    else:
        assert not output.exists()


def test_sbom_is_isolated_and_uploads_do_not_clobber() -> None:
    workflow = yaml.safe_load((ROOT / ".github/workflows/release.yml").read_text())
    jobs = workflow["jobs"]
    assert workflow["permissions"] == {"contents": "read"}
    assert jobs["sbom"]["needs"] == "build"
    assert jobs["sbom"].get("permissions", workflow["permissions"]) == {
        "contents": "read"
    }
    for name, job in jobs.items():
        for step in job["steps"]:
            if step.get("uses", "").startswith("anchore/sbom-action@"):
                assert name == "sbom"
                assert step["with"]["upload-artifact"] is False
                assert step["with"]["upload-release-assets"] is False
                assert step["with"]["output-file"].startswith("sbom/")
    sbom_steps = jobs["sbom"]["steps"]
    assert any(s.get("uses", "").startswith("anchore/sbom-action@") for s in sbom_steps)
    assert not any(
        s.get("uses", "").startswith("actions/download-artifact@") for s in sbom_steps
    )
    assert sbom_steps[0]["with"]["ref"] == "${{ needs.build.outputs.commit }}"
    assert jobs["build"]["outputs"]["commit"] == "${{ steps.tag.outputs.commit }}"
    assembly = jobs["assemble-release-assets"]
    assert set(assembly["needs"]) == {"build", "sbom"}
    downloads = {
        s["with"]["name"]: s["with"]["path"] for s in assembly["steps"]
        if s.get("uses", "").startswith("actions/download-artifact@")
    }
    assert downloads == {"python-distributions": "release/", "release-sbom": "sbom/"}
    for name in ("publish-pypi", "publish-github-assets"):
        assert jobs[name]["needs"] == "assemble-release-assets"
    upload = jobs["publish-github-assets"]["steps"][-1]
    assert "--clobber" not in upload["run"]


def test_all_release_actions_are_commit_pinned() -> None:
    workflow = yaml.safe_load((ROOT / ".github/workflows/release.yml").read_text())
    for job in workflow["jobs"].values():
        for step in job["steps"]:
            if "uses" in step:
                assert re.fullmatch(r"[^@]+@[0-9a-f]{40}", step["uses"])
