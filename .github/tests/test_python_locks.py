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


def test_dependabot_covers_python_manifests() -> None:
    config = yaml.safe_load((ROOT / ".github/dependabot.yml").read_text())
    directories = []
    for update in config["updates"]:
        if update["package-ecosystem"] == "pip":
            assert update["schedule"]["interval"] == "weekly"
            directories.extend(update.get("directories", [update.get("directory")]))
    assert directories and all(isinstance(path, str) for path in directories)
    assert len(directories) == len(set(directories))
    manifests = [
        ROOT / "pyproject.toml", *REQUIREMENTS.glob("*.in"), *REQUIREMENTS.glob("*.txt")
    ]
    manifests.extend((ROOT / "examples").rglob("requirements*.txt"))
    manifests.extend((ROOT / "examples").rglob("requirements*.in"))
    for manifest in manifests:
        # Dependabot's pip fetcher includes files one directory below each root.
        assert any(
            manifest.parent == ROOT / directory.lstrip("/")
            or manifest.parent.parent == ROOT / directory.lstrip("/")
            for directory in directories
        ), str(manifest.relative_to(ROOT))


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
                if "--require-hashes" in args and "--no-index" in args:
                    assert {
                        "--no-index", "--no-deps", "--no-build-isolation", "-r"
                    } <= set(args)
                    assert args[-1] in (
                        "$RUNNER_TEMP/llm-sketchkit-wheel-requirements.txt",
                        "$RUNNER_TEMP/llm-sketchkit-sdist-requirements.txt",
                    )
                elif "--require-hashes" in args:
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
        "${{ inputs.release_tag }}"
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
gh() {
  case "$*" in
    *branches/main*) printf '%s\n' \
      '{"protected":true,"commit":{"sha":"0000000000000000000000000000000000000001"}}'
      ;;
    *releases?*) printf '%s\n' '[[]]' ;;
    *compare/*) printf '%s\n' identical ;;
    *) return 97 ;;
  esac
}
'''
    output = tmp_path / "output"
    result = subprocess.run(
        ["bash", "--noprofile", "--norc", "-eo", "pipefail", "-c",
         lookups + resolve["run"]],
        env={**os.environ, "RELEASE_TAG": tag, "GITHUB_OUTPUT": str(output),
             "GITHUB_REF": f"refs/tags/{tag}", "GITHUB_SHA": f"{1:040d}",
             "WORKFLOW_SHA": f"{1:040d}", "GITHUB_RUN_ATTEMPT": "1",
             "PUBLISH_RELEASE": "false", "PUBLISH_PYPI": "false",
             "GITHUB_REPOSITORY": "llm-measurement/llm-sketchkit",
             "RUNNER_TEMP": str(tmp_path)},
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
        s["with"]["artifact-ids"]: s["with"]["path"] for s in assembly["steps"]
        if s.get("uses", "").startswith("actions/download-artifact@")
    }
    assert downloads == {
        "${{ needs.build.outputs.artifact_id }}": "release/",
        "${{ needs.sbom.outputs.artifact_id }}": "sbom/",
    }
    assert jobs["stage-github-release"]["needs"] == ["build", "assemble-release-assets"]
    assert jobs["publish-pypi"]["needs"] == [
        "build", "assemble-release-assets", "stage-github-release"
    ]
    stage_steps = jobs["stage-github-release"]["steps"]
    stage = next(s["run"] for s in stage_steps if s.get("id") == "draft")
    publish = stage_steps[-1]["run"]
    assert "gh release create" in stage and "--draft" in stage
    assert "--paginate --slurp" in stage
    assert "releases/tags/" not in stage + publish
    assert 'releases/$RELEASE_ID" -F draft=false' in publish
    assert "--release-id" in stage and "--release-id" in publish
    assert "gh release upload" not in stage + publish
    assert "--clobber" not in shlex.split(stage + publish)
    for name in ("publish-pypi", "stage-github-release"):
        download = next(
            s for s in jobs[name]["steps"]
            if s.get("uses", "").startswith("actions/download-artifact@")
        )
        assert download["with"]["artifact-ids"] == (
            "${{ needs.assemble-release-assets.outputs.artifact_id }}"
        )


def test_release_dispatch_defaults_and_public_instructions() -> None:
    workflow = yaml.safe_load((ROOT / ".github/workflows/release.yml").read_text())
    events = workflow.get("on", workflow.get(True))
    assert set(events) == {"workflow_dispatch"}
    inputs = events["workflow_dispatch"]["inputs"]
    assert inputs["publish_release"]["default"] is False
    assert inputs["publish_pypi"]["default"] is False
    source = (ROOT / "docs/SUPPLY_CHAIN.md").read_text()
    assert "exact existing release" in source
    assert "publish_release=true" in source
    assert "publish_pypi=true" in source
    assert "default" in source and "release ID" in source
    locks = (ROOT / "requirements/README.md").read_text()
    assert "dispatch the updated workflow and select" not in locks


@pytest.mark.parametrize(
    "case", ["empty", "yaml_false", "yaml_true", "other_path", "other_word",
             "extra_content", "mixed", "git_error"]
)
def test_repository_leak_guard_retains_all_other_matches(case: str) -> None:
    workflow = yaml.safe_load((ROOT / ".github/workflows/ci.yml").read_text())
    step = next(
        item for item in workflow["jobs"]["go"]["steps"]
        if item.get("name") == "Repository leak guard"
    )
    word = "pro" + "gress"
    key = "cancel-in-" + word
    allowed = f".github/workflows/release.yml:28:  {key}: false"
    cases = {
        "empty": ("", 1, True),
        "yaml_false": (allowed, 0, True),
        "yaml_true": (f".github/workflows/build.yaml:7:  {key}: true", 0, True),
        "other_path": (f"docs/example.yml:1:  {key}: false", 0, False),
        "other_word": (f"README.md:1: {word}", 0, False),
        "extra_content": (allowed + f" # {word}", 0, False),
        "mixed": (allowed + f"\nREADME.md:1: {word}", 0, False),
        "git_error": ("", 2, False),
    }
    matches, status, accepted = cases[case]
    stub = 'git() { printf "%s" "$MATCHES"; return "$GIT_STATUS"; }\n'
    result = subprocess.run(
        ["bash", "--noprofile", "--norc", "-c", stub + step["run"]],
        env={**os.environ, "MATCHES": matches, "GIT_STATUS": str(status)},
        capture_output=True, text=True, check=False,
    )
    assert (result.returncode == 0) is accepted


def test_release_asset_validator_is_in_source_manifest() -> None:
    lines = (ROOT / "MANIFEST.in").read_text().splitlines()
    assert "include scripts/release_assets.py" in lines


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
