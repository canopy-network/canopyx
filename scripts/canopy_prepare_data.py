#!/usr/bin/env python3
"""
Materialize the canopy node ConfigMap contents into a local data directory.

The canopy CLI expects a data directory containing config.json, genesis.json,
keystore.json, and ancillary files. This helper script extracts the embedded
JSON payloads from the Kubernetes ConfigMaps under deploy/k8s/canopy-node/base
so that local tooling (e.g. canopy admin tx-*) can reuse the exact same state.
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path


DEFAULT_CONFIGMAPS = (
    "configmap-config.yaml",
    "configmap-genesis.yaml",
    "configmap-keystore.yaml",
)


def parse_configmap(path: Path) -> dict[str, str]:
    """Parse a very small subset of the ConfigMap YAML structure.

    We only care about the `data` mapping and the block scalar values that follow
    the `key: |` syntax. This avoids pulling in a full YAML parser dependency.
    """

    data: dict[str, str] = {}
    in_data = False
    current_key: str | None = None
    content_indent: int | None = None
    buffer: list[str] = []

    with path.open("r", encoding="utf-8") as handle:
        for raw_line in handle:
            line = raw_line.rstrip("\n")

            if not in_data:
                if line.strip() == "data:":
                    in_data = True
                continue

            if in_data and (line == "" or not line.startswith("  ")):
                if current_key is not None:
                    data[current_key] = "".join(buffer)
                    current_key = None
                    buffer = []
                    content_indent = None
                if line != "":
                    break
                continue

            if current_key is not None:
                if content_indent is None:
                    if not line.strip():
                        buffer.append("\n")
                        continue
                    content_indent = len(raw_line) - len(raw_line.lstrip(" "))
                if line.startswith(" " * content_indent) or not line:
                    slice_from = content_indent if content_indent is not None else 0
                    buffer.append(raw_line[slice_from:])
                    continue
                data[current_key] = "".join(buffer)
                current_key = None
                buffer = []
                content_indent = None

            if not line.strip():
                continue

            stripped = line.strip()
            if ":" not in stripped:
                continue
            key, _, rest = stripped.partition(":")
            rest = rest.strip()

            if rest.startswith("|"):
                current_key = key
                buffer = []
                content_indent = None
            else:
                data[key] = rest

    if current_key is not None:
        data[current_key] = "".join(buffer)

    return data


def write_payload(target: Path, payload: str, force: bool) -> bool:
    if target.exists() and not force:
        return False
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(payload.rstrip() + "\n", encoding="utf-8")
    return True


def materialize(base: Path, output: Path, force: bool) -> list[Path]:
    written: list[Path] = []
    for name in DEFAULT_CONFIGMAPS:
        yaml_path = base / name
        if not yaml_path.exists():
            raise FileNotFoundError(f"ConfigMap file not found: {yaml_path}")
        data = parse_configmap(yaml_path)
        for filename, payload in data.items():
            target = output / filename
            if write_payload(target, payload, force):
                written.append(target)
    return written


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Extract canopy ConfigMap data into a local CLI data directory.",
    )
    parser.add_argument(
        "--base",
        default="deploy/k8s/canopy-node/base",
        help="Directory containing canopy-node ConfigMaps (default: %(default)s).",
    )
    parser.add_argument(
        "--output",
        default="tmp/canopy-local-data",
        help="Destination directory for rendered files (default: %(default)s).",
    )
    parser.add_argument(
        "--force",
        action="store_true",
        help="Overwrite existing files instead of skipping them.",
    )
    return parser.parse_args(argv)


def main(argv: list[str]) -> int:
    args = parse_args(argv)
    base = Path(args.base).resolve()
    output = Path(args.output).resolve()

    if not base.exists():
        raise SystemExit(f"Base directory does not exist: {base}")

    written = materialize(base, output, force=args.force)
    for path in written:
        print(f"[wrote] {path}")
    if not written:
        print(f"No files written (already present) in {output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
