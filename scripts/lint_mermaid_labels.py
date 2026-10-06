#!/usr/bin/env python3
"""Check flowchart node labels for unquoted parentheses."""

import argparse
import re
import sys
from collections.abc import Sequence
from pathlib import Path

PROJECT_ROOT = Path(__file__).resolve().parent.parent
ARCHITECTURE_DIR = PROJECT_ROOT / "architecture"

FLOWCHART_HEADER = re.compile(r"^\s*(?:flowchart|graph)\b", re.IGNORECASE)
NODE_OPEN = re.compile(r"[A-Za-z_][A-Za-z0-9_-]*\[")


def _bracket_label(line: str, start: int) -> tuple[str, int] | None:
    """Return a node label and closing bracket index, ignoring quoted text."""
    quoted = False
    escaped = False
    for index in range(start, len(line)):
        char = line[index]
        if char == '"' and not escaped:
            quoted = not quoted
        elif char == "]" and not quoted:
            return line[start:index], index

        if char == "\\" and not escaped:
            escaped = True
        else:
            escaped = False
    return None


def _has_unquoted_parenthesis(label: str) -> bool:
    quoted = False
    escaped = False
    for char in label:
        if char == '"' and not escaped:
            quoted = not quoted
        elif char in "()" and not quoted:
            return True

        if char == "\\" and not escaped:
            escaped = True
        else:
            escaped = False
    return False


def find_unquoted_parentheses(path: Path) -> list[tuple[int, str]]:
    """Find unquoted parentheses in plain square flowchart node labels.

    Mermaid uses parentheses in flowchart shape syntax. In an unquoted square
    node label, those characters can be tokenized as shape syntax instead of
    label text. Quoted labels and other shapes such as ``ID[(Database)]`` are
    left alone.
    """
    lines = path.read_text(encoding="utf-8").splitlines()
    if not any(FLOWCHART_HEADER.match(line) for line in lines):
        return []

    findings: list[tuple[int, str]] = []
    for line_number, line in enumerate(lines, start=1):
        index = 0
        quoted = False
        escaped = False
        while index < len(line):
            char = line[index]
            if char == '"' and not escaped:
                quoted = not quoted
                index += 1
                escaped = False
                continue

            if not quoted:
                match = NODE_OPEN.match(line, index)
                if match:
                    start = match.end()
                    result = _bracket_label(line, start)
                    if result is not None:
                        label, close_index = result
                        stripped = label.lstrip()
                        is_other_shape = stripped.startswith(("[", "("))
                        if (
                            not is_other_shape
                            and _has_unquoted_parenthesis(label)
                        ):
                            findings.append((line_number, label.strip()))
                        index = close_index + 1
                        escaped = False
                        continue

            if char == "\\" and not escaped:
                escaped = True
            else:
                escaped = False
            index += 1

    return findings


def collect_files(path: Path) -> list[Path]:
    if path.is_file() and path.suffix == ".mmd":
        return [path]
    if path.is_dir():
        return sorted(path.rglob("*.mmd"))
    return []


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "path",
        nargs="?",
        type=Path,
        default=ARCHITECTURE_DIR,
        help=".mmd file or directory to scan (default: %(default)s)",
    )
    args = parser.parse_args(argv)

    files = collect_files(args.path)
    if not files:
        print(f"No Mermaid .mmd files found at: {args.path}", file=sys.stderr)
        return 1

    errors = 0
    for path in files:
        try:
            findings = find_unquoted_parentheses(path)
        except (OSError, UnicodeError) as error:
            print(f"{path}: could not read file: {error}", file=sys.stderr)
            errors += 1
            continue

        for line_number, label in findings:
            print(
                f"{path}:{line_number}: unquoted parentheses in flowchart "
                f"node label {label!r}; quote the complete label, for example "
                'BasicAuth["Basic Auth<br/>(htpasswd)"]'
            )
            errors += 1

    if errors:
        print(f"\n{errors} issue(s) found in {len(files)} Mermaid file(s).")
        return 1

    print(f"Checked {len(files)} Mermaid file(s); no unquoted parentheses found.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
