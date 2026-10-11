# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay and Codex
"""Run README quick starts against separately installed Python and Go packages."""

import argparse
import os
import re
import secrets
import subprocess
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def recipe(document: str, language: str) -> tuple[str, str]:
    section = document.split(f"### {language}\n", 1)[1].split("\n##", 1)[0]
    pattern = (
        r"python - <<'PY'\n(.*?)\nPY" if language == "Python" else r"```go\n(.*?)```"
    )
    code = re.search(pattern, section, re.S)
    output = re.search(r"Output: `([^`]+)`", section)
    if code is None or output is None:
        raise ValueError(f"{language} quick start needs code and documented output")
    return code.group(1), output.group(1) + "\n"


def check(command: list[str], cwd: Path, expected: str) -> None:
    env = {**os.environ, "LLM_SKETCHKIT_SECRET": secrets.token_hex(32),
           "PYTHONPATH": "", "GOWORK": "off"}
    result = subprocess.run(command, cwd=cwd, env=env, check=True,
                            capture_output=True, text=True)
    if result.stdout != expected:
        raise ValueError(f"quick start output {result.stdout!r} != {expected!r}")
    print(result.stdout, end="")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--python", type=Path, help="Python from a fresh package installation",
    )
    parser.add_argument(
        "--go-dir", type=Path, help="fresh Go module with sketchkit installed",
    )
    args = parser.parse_args()
    if args.python is None and args.go_dir is None:
        parser.error("choose --python, --go-dir or both")
    readme = (ROOT / "README.md").read_text()
    if args.python is not None:
        code, output = recipe(readme, "Python")
        with tempfile.TemporaryDirectory(prefix="sketchkit-python-example-") as tmp:
            check([str(args.python.absolute()), "-c", code], Path(tmp), output)
    if args.go_dir is not None:
        code, output = recipe(readme, "Go")
        module = args.go_dir.resolve()
        with tempfile.TemporaryDirectory(prefix="quickstart-", dir=module) as tmp:
            source = Path(tmp) / "main.go"
            source.write_text(code)
            check(["go", "run", str(source)], module, output)


if __name__ == "__main__":
    main()
