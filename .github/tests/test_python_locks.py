# SPDX-License-Identifier: Apache-2.0
from __future__ import annotations

import re
import shlex
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
    assert source_ref == "${{ github.event.release.tag_name || inputs.release_tag }}"
    installs = [
        i for i, step in enumerate(steps) if "pip install" in step.get("run", "")
    ]
    assert len(installs) == 1
    assert tools_index < installs[0] < source_index
    assert "requirements/release.txt" in steps[installs[0]]["run"]
