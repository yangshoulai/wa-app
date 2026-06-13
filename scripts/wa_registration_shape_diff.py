#!/usr/bin/env python3
"""Compare sanitized WA registration field shapes.

The script compares an APK plaintext capture JSON against wa-app runtime shape
logs. It never prints parameter values, only field names, byte lengths and
runtime encoding mode.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from dataclasses import dataclass
from pathlib import Path


SHAPE_LINE_RE = re.compile(r"\bwa_registration_request_shape\b.*?\bfields=(?P<fields>\S+)")
KIND_RE = re.compile(r"\bkind=(?P<kind>\S+)")


@dataclass(frozen=True)
class FieldShape:
    key: str
    length: int
    mode: str = ""


def load_apk_shapes(path: Path) -> list[FieldShape]:
    payload = json.loads(path.read_text())
    params = payload.get("map_params")
    if not isinstance(params, list):
        raise ValueError(f"{path} does not contain list map_params")
    shapes: list[FieldShape] = []
    for item in params:
        key = str(item["key"])
        length = item.get("len")
        if length is None:
            length = len(str(item.get("utf8", "")).encode())
        shapes.append(FieldShape(key=key, length=int(length), mode="apk"))
    return shapes


def load_runtime_shapes(path: Path | None, fields: str | None, kind: str, slice_from: str) -> list[FieldShape]:
    if fields:
        return slice_runtime_fields(parse_runtime_fields(fields), slice_from)
    if path is None:
        raise ValueError("either --runtime-log or --runtime-fields is required")
    selected = ""
    for line in path.read_text(errors="replace").splitlines():
        match = SHAPE_LINE_RE.search(line)
        if not match:
            continue
        kind_match = KIND_RE.search(line)
        if kind_match and kind_match.group("kind") != kind:
            continue
        selected = match.group("fields")
    if not selected:
        raise ValueError(f"no runtime shape line found for kind={kind}")
    return slice_runtime_fields(parse_runtime_fields(selected), slice_from)


def parse_runtime_fields(value: str) -> list[FieldShape]:
    shapes: list[FieldShape] = []
    for chunk in value.strip().split(","):
        if not chunk:
            continue
        parts = chunk.split(":")
        if len(parts) != 3:
            raise ValueError(f"invalid runtime field shape: {chunk}")
        key, length, mode = parts
        shapes.append(FieldShape(key=key, length=int(length), mode=mode))
    return shapes


def slice_runtime_fields(shapes: list[FieldShape], slice_from: str) -> list[FieldShape]:
    if not slice_from:
        return shapes
    for index, shape in enumerate(shapes):
        if shape.key == slice_from:
            return shapes[index:]
    return shapes


def compare(apk: list[FieldShape], runtime: list[FieldShape]) -> int:
    apk_by_key = {field.key: field for field in apk}
    runtime_by_key = {field.key: field for field in runtime}
    apk_keys = [field.key for field in apk]
    runtime_keys = [field.key for field in runtime]

    missing = [key for key in apk_keys if key not in runtime_by_key]
    extra = [key for key in runtime_keys if key not in apk_by_key]
    length_mismatches = [
        (key, apk_by_key[key].length, runtime_by_key[key].length, runtime_by_key[key].mode)
        for key in apk_keys
        if key in runtime_by_key and apk_by_key[key].length != runtime_by_key[key].length
    ]
    order_mismatches = [
        (idx + 1, apk_key, runtime_key)
        for idx, (apk_key, runtime_key) in enumerate(zip(apk_keys, runtime_keys))
        if apk_key != runtime_key
    ]

    print(f"apk_count={len(apk)} runtime_count={len(runtime)}")
    print_list("missing", missing)
    print_list("extra", extra)
    print_order(order_mismatches)
    print_lengths(length_mismatches)
    return 1 if missing or extra or order_mismatches or length_mismatches else 0


def print_list(name: str, values: list[str]) -> None:
    if values:
        print(f"{name}={','.join(values)}")
    else:
        print(f"{name}=<none>")


def print_order(values: list[tuple[int, str, str]]) -> None:
    if not values:
        print("order_mismatch=<none>")
        return
    print("order_mismatch=" + ",".join(f"{idx}:{apk_key}->{runtime_key}" for idx, apk_key, runtime_key in values[:20]))


def print_lengths(values: list[tuple[str, int, int, str]]) -> None:
    if not values:
        print("length_mismatch=<none>")
        return
    print("length_mismatch=" + ",".join(f"{key}:{apk_len}->{runtime_len}:{mode}" for key, apk_len, runtime_len, mode in values))


def main() -> int:
    parser = argparse.ArgumentParser(description="Compare APK and wa-app sanitized registration shapes")
    parser.add_argument("--apk-capture", required=True, type=Path)
    parser.add_argument("--runtime-log", type=Path)
    parser.add_argument("--runtime-fields")
    parser.add_argument("--kind", default="code")
    parser.add_argument("--runtime-slice-from", default="mistyped")
    args = parser.parse_args()

    try:
        apk = load_apk_shapes(args.apk_capture)
        runtime = load_runtime_shapes(args.runtime_log, args.runtime_fields, args.kind, args.runtime_slice_from)
        return compare(apk, runtime)
    except Exception as exc:
        print(f"shape_diff_error={exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
