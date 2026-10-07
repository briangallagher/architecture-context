#!/usr/bin/env python3
"""Validate generated component architecture Markdown files under architecture/."""

import argparse
import asyncio
import sys
from collections.abc import Sequence
from pathlib import Path

PROJECT_ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(PROJECT_ROOT))
ARCHITECTURE_DIR = PROJECT_ROOT / "architecture"

SKIP_NAMES = {"INDEX.md", "PLATFORM.md", "README.md"}

REQUIRED_SECTIONS = [
    "Metadata",
    "Purpose",
    "Architecture Components",
    "APIs Exposed",
    "Dependencies",
    "Network Architecture",
    "Security",
    "Data Flows",
    "Integration Points",
    "Recent Changes",
]


def find_sections(text: str) -> set[str]:
    sections: set[str] = set()
    for line in text.splitlines():
        if line.startswith("## "):
            sections.add(line[3:].strip())
    return sections


def validate_component_doc(path: Path) -> list[str]:
    errors: list[str] = []
    sections = find_sections(path.read_text())
    for section in REQUIRED_SECTIONS:
        if section not in sections:
            errors.append(f"missing required section: ## {section}")
    return errors


def main(
    architecture_dir: Path | None = None,
    arch_analyzer: Path | None = None,
) -> int:
    arch_dir = architecture_dir or ARCHITECTURE_DIR
    if not arch_dir.is_dir():
        print(f"architecture directory not found: {arch_dir}")
        return 1

    from lib.structured_component_publication import has_publication_state

    structured_versions = [
        version
        for version in sorted(arch_dir.iterdir())
        if version.is_dir() and has_publication_state(version)
    ]
    if structured_versions:
        from lib.fetch import _ensure_arch_analyzer
        from lib.structured_component_publication import (
            PublicationError,
            validate_accepted_publications,
        )
        from lib.structured_component_synthesis import GoStructuredAssembler

        analyzer = str(arch_analyzer) if arch_analyzer else asyncio.run(
            _ensure_arch_analyzer()
        )
        assembler = GoStructuredAssembler(
            (analyzer,), Path(__file__).resolve().parents[1] / "src/arch-analyzer"
        )
        try:
            for version in structured_versions:
                validate_accepted_publications(version, assembler)
        except PublicationError as error:
            print(f"invalid accepted structured snapshot: {error}")
            return 1

    seen: set[Path] = set()
    files: list[Path] = []
    for path in sorted(arch_dir.glob("*/*.md")):
        if path.name in SKIP_NAMES:
            continue
        real = path.resolve()
        if real in seen:
            continue
        seen.add(real)
        files.append(path)

    if not files:
        print("No component architecture files found.")
        return 0

    total_errors = 0
    for path in files:
        for error in validate_component_doc(path):
            print(f"{path.relative_to(arch_dir.parent)}: {error}")
            total_errors += 1

    if total_errors:
        print(f"\n{total_errors} error(s) found in {len(files)} file(s)")
        return 1

    print(f"All {len(files)} component architecture file(s) passed validation.")
    return 0


def cli(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--architecture-dir",
        type=Path,
        default=ARCHITECTURE_DIR,
        help="architecture tree to validate (default: %(default)s)",
    )
    parser.add_argument(
        "--arch-analyzer",
        type=Path,
        help="existing arch-analyzer binary (otherwise build the repository binary)",
    )
    args = parser.parse_args(argv)
    return main(args.architecture_dir, args.arch_analyzer)


if __name__ == "__main__":
    sys.exit(cli())
