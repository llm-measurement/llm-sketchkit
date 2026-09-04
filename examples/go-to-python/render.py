# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay Erramilli and Codex
"""Execute the source notebook and save its two actual plots for documentation."""

import base64
from pathlib import Path

import nbformat
from nbclient import NotebookClient


def main():
    root = Path(__file__).resolve().parents[2]
    notebook = nbformat.read(
        root / "examples/go-to-python/go-to-python.ipynb", as_version=4
    )
    assert all(not cell.get("outputs") for cell in notebook.cells)
    NotebookClient(
        notebook,
        timeout=180,
        kernel_name="python3",
        resources={"metadata": {"path": str(root)}},
    ).execute()
    plots = [
        output["data"]["image/png"]
        for cell in notebook.cells
        for output in cell.get("outputs", [])
        if "image/png" in output.get("data", {})
    ]
    if len(plots) != 2:
        raise RuntimeError(f"expected two notebook plots, found {len(plots)}")
    target = root / "docs/images"
    target.mkdir(parents=True, exist_ok=True)
    for name, encoded in zip(
        ("distinct-users.png", "token-bounds.png"), plots, strict=True
    ):
        (target / name).write_bytes(base64.b64decode(encoded, validate=True))
        print(f"docs/images/{name}")


if __name__ == "__main__":
    main()
