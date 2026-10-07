"""Phase 6: Generate architecture diagrams."""

import json
import os
import tempfile
from pathlib import Path

from lib.agent_runner import run_agents_concurrently
from lib.fetch import _ensure_arch_analyzer
from lib.structured_component_publication import (
    accepted_publications,
    has_publication_state,
)
from lib.structured_component_reuse import content_hash
from lib.structured_component_synthesis import GoStructuredAssembler


def _write_diagram_metadata(path: Path, value: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary = tempfile.mkstemp(prefix=".metadata.", dir=path.parent)
    try:
        with os.fdopen(descriptor, "w") as stream:
            json.dump(value, stream, indent=2, sort_keys=True)
            stream.write("\n")
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
    except BaseException:
        Path(temporary).unlink(missing_ok=True)
        raise


async def run_generate_diagrams_phase(args) -> None:
    """Run Phase 6: Generate diagrams for architecture files."""
    print("\n" + "=" * 60)
    print("PHASE 6: Generating architecture diagrams")
    print("=" * 60 + "\n")

    architecture_dir = Path(args.architecture_dir)
    harness = getattr(args, "harness", "claude")
    selected_model = args.model or (
        "opus" if harness == "claude" else "configured default"
    )
    generator_identity = f"{harness}:{selected_model}:architecture-diagram/v1"

    if not architecture_dir.exists():
        print(f"Error: Architecture directory does not exist: {architecture_dir}")
        return

    # Determine which directories to scan
    if args.platform:
        candidate = architecture_dir / args.platform
        if not candidate.is_dir():
            print(f"No directory found: {candidate}")
            available = [
                p.name for p in sorted(architecture_dir.iterdir())
                if p.is_dir()
            ]
            print(f"Available: {', '.join(available)}")
            return
        scan_dirs = [candidate]
    else:
        scan_dirs = sorted(d for d in architecture_dir.iterdir() if d.is_dir())

    if args.version:
        scan_dirs = [d for d in scan_dirs if args.version in d.name]
        if not scan_dirs:
            print(f"No directories found matching version: {args.version}")
            return

    # Discover all .md files in selected directories
    diagram_jobs = []

    analyzer_assembler = None
    for platform_dir in scan_dirs:
        accepted = {}
        if has_publication_state(platform_dir):
            if analyzer_assembler is None:
                analyzer_assembler = GoStructuredAssembler(
                    (await _ensure_arch_analyzer(),),
                    Path(__file__).resolve().parents[2] / "src/arch-analyzer",
                )
            accepted = accepted_publications(
                platform_dir, analyzer_assembler, repair_markdown=True
            )
        # Find all .md files (excluding README.md)
        md_files = [
            f for f in platform_dir.glob("*.md")
            if f.name not in ("INDEX.md", "README.md")
        ]

        # Filter to a single component if --component was provided
        if args.component:
            target = args.component.lower()
            md_files = [f for f in md_files if f.stem.lower() == target]

        for md_file in md_files:
            component_name = md_file.stem.lower()
            accepted_snapshot = accepted.get(component_name)
            document_hash = (
                content_hash(accepted_snapshot.document)
                if accepted_snapshot is not None
                else None
            )
            diagrams_dir = (
                platform_dir / component_name / "diagrams"
                if accepted_snapshot is not None
                else platform_dir / "diagrams"
            )

            # Check if diagrams already exist
            has_diagrams = False
            all_diagram_files = []
            if diagrams_dir.exists():
                mmd_files = list(diagrams_dir.glob(f"{component_name}-*.mmd"))
                dsl_files = list(diagrams_dir.glob(f"{component_name}-*.dsl"))
                txt_files = list(diagrams_dir.glob(f"{component_name}-*.txt"))
                png_files = list(diagrams_dir.glob(f"{component_name}-*.png"))
                all_diagram_files = mmd_files + dsl_files + txt_files + png_files
                has_diagrams = (
                    len(mmd_files) > 0
                    or len(dsl_files) > 0
                    or len(txt_files) > 0
                )
                if has_diagrams and document_hash is not None:
                    try:
                        metadata = json.loads(
                            (diagrams_dir / "metadata.json").read_text()
                        )
                    except (OSError, json.JSONDecodeError):
                        metadata = {}
                    if (
                        metadata.get("document_hash") != document_hash
                        or metadata.get("generator_identity") != generator_identity
                        or metadata.get("state") != "available"
                    ):
                        for diagram_file in all_diagram_files:
                            diagram_file.unlink()
                        has_diagrams = False

            needs_generation = False
            if not has_diagrams:
                needs_generation = True
            elif args.force_regenerate:
                print(
                    f"  Deleting existing diagrams for"
                    f" {component_name} (--force-regenerate)"
                )
                for diagram_file in all_diagram_files:
                    diagram_file.unlink()
                needs_generation = True
                has_diagrams = False
            elif has_diagrams:
                md_mtime = md_file.stat().st_mtime
                oldest_diagram_mtime = min(f.stat().st_mtime for f in all_diagram_files)
                if md_mtime > oldest_diagram_mtime:
                    print(
                        f"  Deleting stale diagrams for"
                        f" {component_name} (source file updated)"
                    )
                    for diagram_file in all_diagram_files:
                        diagram_file.unlink()
                    needs_generation = True
                    has_diagrams = False

            diagram_jobs.append({
                'dir_name': platform_dir.name,
                'md_file': md_file,
                'component_name': component_name,
                'diagrams_dir': diagrams_dir,
                'has_diagrams': has_diagrams,
                'needs_generation': needs_generation,
                'document_hash': document_hash,
            })

    if not diagram_jobs:
        if args.component:
            print(f"No architecture file found for component '{args.component}'")
            # Show available components in scanned dirs
            available = []
            for d in scan_dirs:
                available.extend(
                    f.stem for f in d.glob("*.md")
                    if f.name not in ("INDEX.md", "README.md")
                )
            if available:
                print(f"Available components: {', '.join(sorted(available))}")
        else:
            print(f"No architecture files found in {architecture_dir}")
        return

    # Filter to files that need diagrams
    needs_generation = [j for j in diagram_jobs if j['needs_generation']]
    already_has = [
        j for j in diagram_jobs
        if j['has_diagrams'] and not args.force_regenerate
    ]

    print(f"Found {len(diagram_jobs)} architecture file(s):")
    print(f"  Already has diagrams: {len(already_has)}")
    print(f"  Needs diagrams: {len(needs_generation)}")
    print()

    if already_has:
        print("Files with diagrams (skipping):")
        for j in already_has:
            print(f"  + {j['dir_name']}/{j['component_name']}.md")
        print()

    if not needs_generation:
        print("All files already have diagrams!")
        if not args.force_regenerate:
            print("Use --force-regenerate to regenerate existing diagrams")
        return

    # Separate PLATFORM files from component files
    # (different skills, different priority)
    platform_jobs_data = [
        j for j in needs_generation
        if j['component_name'] == 'platform'
    ]
    component_jobs_data = [
        j for j in needs_generation
        if j['component_name'] != 'platform'
    ]

    export_png = getattr(args, 'export_png', False)

    def _build_jobs(items, skill):
        jobs = []
        for j in sorted(items, key=lambda x: (x['dir_name'], x['component_name'])):
            arch_flag = (
                'platform-file'
                if skill == 'generate-platform-diagrams'
                else 'architecture'
            )
            md_path = j['md_file'].resolve()
            out_path = j['diagrams_dir'].resolve()
            prompt = (
                f"/{skill}"
                f" --{arch_flag}={md_path}"
                f" --output-dir={out_path}"
            )
            if export_png:
                prompt += " --export-png"
            jobs.append({
                "name": f"{j['dir_name']}/{j['component_name']}",
                "cwd": ".",
                "prompt": prompt,
                "component_name": j['component_name'],
                "diagrams_dir": j['diagrams_dir'],
                "document_hash": j['document_hash'],
            })
        return jobs

    platform_jobs = _build_jobs(platform_jobs_data, "generate-platform-diagrams")
    component_jobs = _build_jobs(component_jobs_data, "generate-architecture-diagrams")
    jobs = platform_jobs + component_jobs

    # Apply limit if specified
    if args.limit:
        jobs = jobs[:args.limit]
        platform_jobs = [j for j in jobs if j['component_name'] == 'platform']
        component_jobs = [j for j in jobs if j['component_name'] != 'platform']
        print(f"Limited to first {args.limit} file(s)\n")

    # Display prepared jobs
    print(f"Prepared {len(jobs)} agent job(s):\n")
    if platform_jobs:
        print("Platform diagrams (run first):")
        for i, job in enumerate(platform_jobs, 1):
            print(f"  {i:2d}. {job['name']}")
        print()
    if component_jobs:
        print("Component diagrams:")
        for i, job in enumerate(component_jobs, 1):
            print(f"  {i:2d}. {job['name']}")
    print()

    # Create logs directory
    log_dir = Path("logs/generate-diagrams")
    log_dir.mkdir(parents=True, exist_ok=True)
    print(f"Logs will be written to: {log_dir}\n")

    print(f"{'=' * 60}")
    print(f"Ready to process {len(jobs)} file(s)")
    print(f"Max concurrent agents: {args.max_concurrent}")
    print(f"Harness: {harness}")
    print(f"Model: {selected_model}")
    print(f"{'=' * 60}\n")

    strace_prefix = (
        f"{args.platform}-generate-diagrams"
        if getattr(args, 'strace', False) else None
    )

    # Run platform diagrams first, then component diagrams
    all_results = []
    all_jobs = []

    if platform_jobs:
        print("--- Phase 6a: Platform diagrams ---\n")
        platform_results = await run_agents_concurrently(
            platform_jobs, log_dir, args.model, args.max_concurrent,
            enable_skills=True, strace_prefix=strace_prefix,
            phase_label="PHASE 6a · Platform diagram generation",
            harness=harness,
        )
        all_results.extend(platform_results)
        all_jobs.extend(platform_jobs)

    if component_jobs:
        print("\n--- Phase 6b: Component diagrams ---\n")
        component_results = await run_agents_concurrently(
            component_jobs, log_dir, args.model,
            args.max_concurrent, enable_skills=True,
            strace_prefix=strace_prefix,
            phase_label="PHASE 6b · Component diagram generation",
            harness=harness,
        )
        all_results.extend(component_results)
        all_jobs.extend(component_jobs)

    for job, result in zip(all_jobs, all_results):
        if job.get("document_hash") is None:
            continue
        produced = any(
            path.suffix in {".mmd", ".dsl", ".txt", ".png"}
            for path in job["diagrams_dir"].glob(f"{job['component_name']}-*")
        )
        state = (
            "available"
            if isinstance(result, dict) and result.get("success") and produced
            else "failed"
        )
        _write_diagram_metadata(
            job["diagrams_dir"] / "metadata.json",
            {
                "schema_version": "1.0.0",
                "document_hash": job["document_hash"],
                "generator_identity": generator_identity,
                "state": state,
            },
        )

    # Summary
    successful = [r for r in all_results if isinstance(r, dict) and r.get("success")]
    failed = [r for r in all_results if isinstance(r, dict) and not r.get("success")]
    exceptions = [r for r in all_results if isinstance(r, Exception)]

    print("\n" + "=" * 60)
    print("DIAGRAM GENERATION COMPLETE")
    print("=" * 60)
    print(f"Total files: {len(all_jobs)}")
    print(f"Successful: {len(successful)}")
    print(f"Failed: {len(failed)}")
    if exceptions:
        print(f"Exceptions: {len(exceptions)}")

    if failed:
        print("\nFailed files:")
        for r in failed:
            print(f"  x {r['name']}: {r.get('error', 'unknown error')}")
            if r.get('log_file'):
                print(f"    Log: {r['log_file']}")

    if exceptions:
        print("\nFiles with exceptions:")
        for i, exc in enumerate(exceptions):
            print(f"  x Exception {i+1}: {exc}")

    print(f"\nAll agent logs available in: {log_dir}")
    print("=" * 60)
