# SPDX-License-Identifier: Apache-2.0
from __future__ import annotations

import os
import re
import shlex
import subprocess
import tomllib
from pathlib import Path

import pytest
import yaml
from packaging.markers import default_environment
from packaging.requirements import Requirement
from packaging.utils import canonicalize_name

ROOT = Path(__file__).resolve().parents[2]
REQUIREMENTS = ROOT / "requirements"
PROFILES = ("runtime", "dev", "notebook", "proto", "release", "lock")


def locked_requirements(profile: str) -> list[Requirement]:
    text = (REQUIREMENTS / f"{profile}.txt").read_text()
    assert text.startswith("# SPDX-License-Identifier: Apache-2.0\n")
    requirements = []
    for line in text.replace("\\\n", " ").splitlines():
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        requirement, *hashes = re.split(r"\s+--hash=", line.strip())
        assert hashes and all(re.fullmatch(r"sha256:[0-9a-f]{64}", h) for h in hashes)
        parsed = Requirement(requirement)
        assert parsed.url is None
        assert len(parsed.specifier) == 1
        pin = next(iter(parsed.specifier))
        assert pin.operator == "==" and "*" not in pin.version
        requirements.append(parsed)
    assert requirements
    return requirements


def input_requirements(path: Path) -> list[Requirement]:
    requirements = []
    for line in path.read_text().splitlines():
        if line.startswith("-r "):
            requirements.extend(input_requirements(path.parent / line[3:]))
        elif line and not line.startswith("#"):
            requirements.append(Requirement(line))
    return requirements


@pytest.mark.parametrize("profile", PROFILES)
@pytest.mark.parametrize("minor", (11, 12, 13, 14))
def test_locks_cover_inputs_and_package_metadata(profile: str, minor: int) -> None:
    environment = default_environment()
    environment.update(
        python_version=f"3.{minor}",
        python_full_version=f"3.{minor}.0",
        sys_platform="linux",
        os_name="posix",
        platform_machine="x86_64",
        implementation_name="cpython",
        platform_python_implementation="CPython",
    )
    active = [
        req
        for req in locked_requirements(profile)
        if req.marker is None or req.marker.evaluate(environment)
    ]
    pins = {
        canonicalize_name(req.name): next(iter(req.specifier)).version for req in active
    }
    assert len(pins) == len(active)
    expected = input_requirements(REQUIREMENTS / f"{profile}.in")
    metadata = tomllib.loads((ROOT / "pyproject.toml").read_text())
    if profile != "lock":
        expected.extend(map(Requirement, metadata["build-system"]["requires"]))
    if profile in ("runtime", "dev", "notebook"):
        expected.extend(map(Requirement, metadata["project"]["dependencies"]))
    if profile in ("dev", "release"):
        expected.extend(
            map(Requirement, metadata["project"]["optional-dependencies"][profile])
        )
    for req in expected:
        if req.marker is None or req.marker.evaluate(environment):
            assert pins[canonicalize_name(req.name)] in req.specifier, str(req)


@pytest.mark.parametrize("name", ("ci", "fuzz", "release"))
def test_workflows_only_install_locked_or_local_packages(name: str) -> None:
    workflow = yaml.safe_load((ROOT / f".github/workflows/{name}.yml").read_text())
    installs = 0
    for job in workflow["jobs"].values():
        for step in job["steps"]:
            for line in step.get("run", "").replace("\\\n", " ").splitlines():
                if " -m build" in line:
                    assert "--no-isolation" in shlex.split(line)
                if " -m pip_audit" in line:
                    assert {"--disable-pip", "--require-hashes", "-r"} <= set(
                        shlex.split(line)
                    )
                if not re.search(r"\bpip(?:\d+(?:\.\d+)?)? install\b", line):
                    continue
                installs += 1
                args = shlex.split(line)
                if "--require-hashes" in args:
                    assert "--only-binary=:all:" in args
                    path = Path(args[args.index("-r") + 1])
                    assert path.parent == Path("requirements")
                    assert path.stem in PROFILES and path.suffix == ".txt"
                    assert (ROOT / path).is_file()
                else:
                    assert {"--no-index", "--no-deps", "--no-build-isolation"} <= set(
                        args
                    )
                    assert args[-1] in (".", ".[dev]", "$wheel", "${sdist}[dev]")
    assert installs


def test_release_installs_workflow_locks_before_selecting_tag() -> None:
    workflow = yaml.safe_load((ROOT / ".github/workflows/release.yml").read_text())
    steps = workflow["jobs"]["build"]["steps"]
    checkouts = [
        (i, step["with"]["ref"])
        for i, step in enumerate(steps)
        if step.get("uses", "").startswith("actions/checkout@")
    ]
    assert len(checkouts) == 2
    (tools_index, tools_ref), (source_index, source_ref) = checkouts
    assert tools_ref == "${{ github.workflow_sha }}"
    assert source_ref == "${{ steps.tag.outputs.commit }}"
    installs = [
        i for i, step in enumerate(steps) if "pip install" in step.get("run", "")
    ]
    assert len(installs) == 1
    assert tools_index < installs[0] < source_index
    assert "requirements/release.txt" in steps[installs[0]]["run"]


@pytest.mark.parametrize(
    ("tag", "accepted"),
    [
        ("v0.2.2", True), ("v0.3.0-alpha.1", True),
        ("main", False), ("4ca69f8", False), ("refs/tags/v0.2.2", False),
        ("vbranch", False), ("vmissing", False), ("vnotcommit", False),
        ("v0.2.2^{commit}", False), ("v0.2.2\ncommit=forged", False),
        ("v0.2.2; exit 0", False), ("", False),
    ],
)
def test_release_source_requires_existing_version_tag(
    tmp_path: Path, tag: str, accepted: bool,
) -> None:
    workflow = yaml.safe_load((ROOT / ".github/workflows/release.yml").read_text())
    steps = workflow["jobs"]["build"]["steps"]
    resolve_index, resolve = next(
        (i, step) for i, step in enumerate(steps) if step.get("id") == "tag"
    )
    assert steps[0]["with"]["fetch-depth"] == 0
    assert resolve_index < next(
        i for i, step in enumerate(steps) if "pip install" in step.get("run", "")
    )
    assert "${{" not in resolve["run"]
    assert resolve["env"]["RELEASE_TAG"] == (
        "${{ github.event.release.tag_name || inputs.release_tag }}"
    )
    # Exercise the actual shell guard without creating tags or contacting GitHub.
    # Git itself checks syntax; these two read-only lookups model fetched tags.
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


def test_release_sbom_is_isolated_and_uploads_do_not_clobber() -> None:
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
    assert all(
        jobs[name]["needs"] == "assemble-release-assets"
        for name in ("publish-pypi", "publish-github-assets")
    )
    upload = jobs["publish-github-assets"]["steps"][-1]["run"]
    assert "gh release upload" in upload
    assert "--clobber" not in shlex.split(upload)


def test_codeowners_cover_decoder_and_dependency_inputs() -> None:
    lines = (ROOT / ".github/CODEOWNERS").read_text().splitlines()
    owned = {
        line.split()[0] for line in lines
        if line and not line.startswith("#")
    }
    assert {"/requirements/", "/go.sum", "/spec/sketches.proto"} <= owned
    for kind in ("bloom", "frequentitems", "hllpp", "minhash", "summary"):
        assert f"/go/sketchkit/{kind}/{kind}.go" in owned
        assert f"/python/llm_sketchkit/{kind}.py" in owned
    assert "/python/llm_sketchkit/_proto.py" in owned
    assert "/python/llm_sketchkit/sketches_pb2.py" in owned
    assert "/go/sketchkit/internal/pb/" in owned
