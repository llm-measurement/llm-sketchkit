# Code authors: Vijay Erramilli and Codex
"""Render deterministic, dependency-free SVG charts from scorecard.json."""

from __future__ import annotations

import argparse
import json
import xml.etree.ElementTree as ET
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / "reports" / "scorecard.json"
ASSETS = ROOT / "reports" / "assets"

SVG_NS = "http://www.w3.org/2000/svg"
WIDTH = 760
LEFT = 180
RIGHT = 40
ROW_HEIGHT = 48
TEXT = "#17202a"
MUTED = "#5f6b76"
GRID = "#d7dde3"
BLUE = "#1976a3"
GREEN = "#27864b"
GOLD = "#b7791f"

ET.register_namespace("", SVG_NS)


def tag(name: str) -> str:
    return f"{{{SVG_NS}}}{name}"


def new_chart(title: str, subtitle: str, height: int) -> ET.Element:
    root = ET.Element(
        tag("svg"),
        {
            "width": str(WIDTH),
            "height": str(height),
            "viewBox": f"0 0 {WIDTH} {height}",
            "role": "img",
            "aria-labelledby": "title desc",
        },
    )
    ET.SubElement(root, tag("title"), {"id": "title"}).text = title
    ET.SubElement(root, tag("desc"), {"id": "desc"}).text = subtitle
    ET.SubElement(
        root,
        tag("rect"),
        {"width": str(WIDTH), "height": str(height), "fill": "#ffffff"},
    )
    add_text(root, 24, 32, title, size=18, weight="700")
    add_text(root, 24, 54, subtitle, fill=MUTED, size=12)
    return root


def add_text(
    root: ET.Element,
    x: float,
    y: float,
    value: object,
    *,
    fill: str = TEXT,
    size: int = 13,
    weight: str | None = None,
) -> None:
    attributes = {
        "x": format_number(x),
        "y": format_number(y),
        "fill": fill,
        "font-family": "system-ui, sans-serif",
        "font-size": str(size),
    }
    if weight is not None:
        attributes["font-weight"] = weight
    ET.SubElement(root, tag("text"), attributes).text = str(value)


def add_line(
    root: ET.Element,
    x1: float,
    y1: float,
    x2: float,
    y2: float,
    *,
    stroke: str,
    width: int,
) -> None:
    ET.SubElement(
        root,
        tag("line"),
        {
            "x1": format_number(x1),
            "y1": format_number(y1),
            "x2": format_number(x2),
            "y2": format_number(y2),
            "stroke": stroke,
            "stroke-width": str(width),
        },
    )


def add_key(root: ET.Element, x: int, label: str, color: str) -> None:
    ET.SubElement(
        root,
        tag("rect"),
        {"x": str(x), "y": "68", "width": "10", "height": "10", "fill": color},
    )
    add_text(root, x + 16, 77, label, fill=MUTED, size=11)


def format_number(value: float) -> str:
    return f"{value:.2f}".rstrip("0").rstrip(".")


def serialize(root: ET.Element) -> str:
    ET.indent(root, space="  ")
    return ET.tostring(root, encoding="unicode", short_empty_elements=True) + "\n"


def bars(
    title: str,
    subtitle: str,
    rows: list[tuple[str, float, str]],
    maximum: float,
    formatter: str,
) -> str:
    chart_width = WIDTH - LEFT - RIGHT
    root = new_chart(title, subtitle, 80 + ROW_HEIGHT * len(rows))
    for index, (label, value, color) in enumerate(rows):
        y = 80 + index * ROW_HEIGHT
        bar_width = chart_width * value / maximum
        add_text(root, 24, y + 15, label)
        add_line(
            root,
            LEFT,
            y + 10,
            WIDTH - RIGHT,
            y + 10,
            stroke=GRID,
            width=18,
        )
        add_line(
            root,
            LEFT,
            y + 10,
            LEFT + bar_width,
            y + 10,
            stroke=color,
            width=18,
        )
        add_text(
            root,
            LEFT + bar_width + 8,
            y + 15,
            formatter.format(value),
            size=12,
            weight="700",
        )
    return serialize(root)


def grouped_bars(
    title: str,
    subtitle: str,
    rows: list[tuple[str, float, float]],
    maximum: float,
    labels: tuple[str, str],
    formatter: str,
) -> str:
    chart_width = WIDTH - LEFT - RIGHT
    root = new_chart(title, subtitle, 92 + 68 * len(rows))
    add_key(root, LEFT, labels[0], BLUE)
    add_key(root, LEFT + 130, labels[1], GOLD)
    for index, (label, first, second) in enumerate(rows):
        y = 98 + index * 68
        add_text(root, 24, y + 20, label)
        for offset, value, color in ((5, first, BLUE), (31, second, GOLD)):
            bar_width = chart_width * value / maximum
            add_line(
                root,
                LEFT,
                y + offset,
                WIDTH - RIGHT,
                y + offset,
                stroke=GRID,
                width=14,
            )
            add_line(
                root,
                LEFT,
                y + offset,
                LEFT + bar_width,
                y + offset,
                stroke=color,
                width=14,
            )
            add_text(
                root,
                LEFT + bar_width + 6,
                y + offset + 4,
                formatter.format(value),
                size=11,
            )
    return serialize(root)


def render(data: dict[str, Any]) -> dict[str, str]:
    performance = [
        (row["label"], float(row["multiple"]), GREEN)
        for row in data["linux_benchmark"]["headroom"]
    ]
    hllpp = [
        (row["label"], float(row["percent"]), BLUE if index == 0 else GOLD)
        for index, row in enumerate(data["hllpp_small"])
    ]
    bloom = [
        (row["label"], float(row["observed"]), float(row["target"]))
        for row in data["bloom"]
    ]
    minhash = [
        (row["label"], float(row["mean"]), float(row["p95"]))
        for row in data["minhash"]
    ]
    return {
        "performance-headroom.svg": bars(
            "Linux Performance Headroom",
            "Least favorable of five runs; 1x meets the stated target",
            performance,
            50.0,
            "{:.2f}x",
        ),
        "hllpp-error.svg": bars(
            "HLL++ Small-Profile Relative Error",
            "Maximum observed across the grid compared with the enforced bound",
            hllpp,
            2.6,
            "{:.3f}%",
        ),
        "bloom-fpr.svg": grouped_bars(
            "Bloom False-Positive Rate",
            "Empirical rate compared with each profile's configured target",
            bloom,
            0.0011,
            ("Observed", "Target"),
            "{:.6f}",
        ),
        "minhash-error.svg": grouped_bars(
            "MinHash Absolute Error",
            "Mean and p95 over 1,000 deterministic set pairs",
            minhash,
            0.08,
            ("Mean", "p95"),
            "{:.5f}",
        ),
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--check",
        action="store_true",
        help="fail if checked-in charts differ from generated output",
    )
    args = parser.parse_args()

    data = json.loads(SOURCE.read_text(encoding="utf-8"))
    outputs = render(data)
    if args.check:
        stale = [
            name
            for name, content in outputs.items()
            if not (ASSETS / name).is_file()
            or (ASSETS / name).read_text(encoding="utf-8") != content
        ]
        if stale:
            parser.error("stale scorecard assets: " + ", ".join(stale))
        return 0

    ASSETS.mkdir(parents=True, exist_ok=True)
    for name, content in outputs.items():
        (ASSETS / name).write_text(content, encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
