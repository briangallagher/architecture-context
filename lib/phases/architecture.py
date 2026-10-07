"""Phase 3: Generate component architecture documentation."""

import shutil
from pathlib import Path

from lib.agent_runner import (
    get_model_display_name,
    run_agents_concurrently,
)
from lib.cli import resolve_distribution
from lib.component_discovery import (
    apply_component_selection,
    apply_platform_overrides,
    get_component_map_metadata,
    read_component_map,
)
from lib.fetch import load_platform_config
from lib.phases.static_analysis import analyzer_output_dir
from lib.repo_naming import extra_repo_checkout_name


def component_output_path(
    architecture_dir: str | Path, platform: str, component_key: str,
) -> Path:
    """Return the canonical generated document path for one component."""
    return (Path(architecture_dir) / platform / f"{component_key}.md").resolve()


def component_generation_dir(
    architecture_dir: str | Path, platform: str, component_key: str,
) -> Path:
    """Return the private sidecar directory for one generation run."""
    return (
        Path(architecture_dir) / platform / component_key / ".generation"
    ).resolve()


def component_generation_path(
    architecture_dir: str | Path,
    platform: str,
    component_key: str,
    filename: str,
) -> Path:
    """Return one private generation artifact path for a component."""
    return component_generation_dir(
        architecture_dir, platform, component_key,
    ) / filename


def _remove_legacy_component_outputs(
    architecture_dir: str | Path,
    platform: str,
    platform_config: dict,
    components,
) -> None:
    """Remove generated artifacts left under a component's former raw name.

    ``name_prefix`` changes the component key used by every downstream phase.
    Remove the old generated names during the transition so platform synthesis
    and diagram discovery cannot see both ``policy`` and ``praxis-policy``.
    """
    platform_dir = Path(architecture_dir) / platform
    diagrams_dir = platform_dir / "diagrams"
    for component in components.values():
        alias = extra_repo_checkout_name(
            platform_config, component.repo_org, component.repo_name,
        )
        if not alias or alias == component.repo_name or not component.repo_name:
            continue
        if component.repo_name in components:
            continue

        legacy_file = platform_dir / f"{component.repo_name}.md"
        if legacy_file.exists():
            legacy_file.unlink()
            print(f"  Removed legacy component document: {legacy_file}")

        for legacy_dir in (
            platform_dir / component.repo_name / ".analyzer",
            platform_dir / component.repo_name / ".generation",
        ):
            if legacy_dir.exists():
                shutil.rmtree(legacy_dir)
                print(f"  Removed legacy component artifacts: {legacy_dir}")
        legacy_component_dir = platform_dir / component.repo_name
        if legacy_component_dir.is_dir() and not any(legacy_component_dir.iterdir()):
            legacy_component_dir.rmdir()

        if diagrams_dir.exists():
            active_prefixes = tuple(
                f"{k}-" for k in components if k != component.repo_name
            )
            legacy_prefix = f"{component.repo_name}-"
            for diagram_file in diagrams_dir.iterdir():
                if not diagram_file.is_file():
                    continue
                name = diagram_file.name
                if not name.startswith(legacy_prefix):
                    continue
                if any(name.startswith(p) for p in active_prefixes):
                    continue
                diagram_file.unlink()
                print(f"  Removed legacy diagram: {diagram_file}")


async def run_generate_architecture_phase(args) -> None:
    """Run Phase 3: Generate architecture documentation."""
    print("\n" + "=" * 60)
    print("PHASE 3: Generating component architectures")
    print("=" * 60 + "\n")

    architecture_dir = getattr(args, "architecture_dir", "architecture")
    distribution = resolve_distribution(args.platform)

    # Load components from component-map.json
    components = read_component_map(args.platform, architecture_dir=architecture_dir)
    if components is None:
        print(f"ERROR: No component-map.json found for platform '{args.platform}'")
        print(f"Expected: {architecture_dir}/{args.platform}/component-map.json")
        print("\nRun discover-components first:")
        print(f"  uv run main.py discover-components --platform={args.platform}")
        return

    # Apply platform overrides (exclude_components, include_components, etc.)
    platform_config = load_platform_config(
        args.platform, getattr(args, "platforms_file", "platforms.yaml")
    )
    if platform_config:
        checkouts_dir = getattr(args, "checkouts_dir", "checkouts")
        components = apply_platform_overrides(
            components,
            platform_config,
            checkouts_base=checkouts_dir,
        )
        _remove_legacy_component_outputs(
            architecture_dir, args.platform, platform_config, components,
        )
    components = apply_component_selection(
        components,
        get_component_map_metadata(args.platform, architecture_dir),
    )

    if not components:
        print("No components found with checkouts")
        return

    # Apply component filter if specified
    if args.component:
        if args.component in components:
            components = {args.component: components[args.component]}
            print(f"Filtered to single component: {args.component}\n")
        else:
            print(f"ERROR: Component '{args.component}' not found")
            print(f"Available components: {', '.join(sorted(components.keys()))}")
            return

    # Filter to components with actual checkouts on disk
    components = {
        k: v
        for k, v in components.items()
        if v.checkout_path and v.checkout_path.exists()
    }

    # Apply tier filter
    tier_filter = getattr(args, "tier", "all")
    if tier_filter == "significant":
        before = len(components)
        components = {
            k: v for k, v in components.items() if v.architecturally_significant
        }
        print(f"Tier filter 'significant': {before} -> {len(components)} components")
    elif tier_filter == "core":
        before = len(components)
        components = {
            k: v
            for k, v in components.items()
            if v.tier in ("core_platform", "optional_platform")
        }
        print(f"Tier filter 'core': {before} -> {len(components)} components")

    # Refresh has_architecture from the canonical architecture output tree.
    for component in components.values():
        arch_file = component_output_path(
            architecture_dir, args.platform, component.key,
        )
        component.has_architecture = arch_file.exists()

    if args.force:
        missing_arch = [c for c in components.values()]
    else:
        missing_arch = [c for c in components.values() if not c.has_architecture]
    has_arch = [c for c in components.values() if c.has_architecture]

    print(f"Found {len(components)} components:")
    print(f"  Already documented: {len(has_arch)}")
    print(f"  Need architecture: {len(missing_arch)}")
    print()

    if not missing_arch and not args.force:
        print("All components already have architecture documentation!")
        return

    harness = getattr(args, "harness", "claude")
    model_display = (
        get_model_display_name(args.model, harness=harness)
        if harness != "claude"
        else get_model_display_name(args.model)
    )

    work_items = []
    for component in sorted(missing_arch, key=lambda c: c.key):
        analyzer_root = analyzer_output_dir(
            architecture_dir, args.platform, component.key,
        )
        checkout_path = str(component.checkout_path.resolve())
        final_output_path = component_output_path(
            architecture_dir, args.platform, component.key,
        )
        if not component.lineage:
            lineage=""
        else:
            lineage = ",".join(component.lineage)
        prompt = (
            f"/repo-to-architecture-summary {checkout_path}"
            f" --analyzer-dir={analyzer_root}"
            f" --generation-dir={analyzer_root}"
            f" --distribution={distribution}"
            f" --platform={distribution}"
            f" --output={final_output_path}"
            f" --generated-by={model_display}"
            f" --component-name={model_display}"
            f" --lineage={lineage}"
        )

        job = {
            "name": f"{component.key}",
            "cwd": ".",
            "prompt": prompt,
            "repo": f"{component.repo_org}/{component.repo_name}",
            "checkout_path": component.checkout_path,
            "analyzer_root": analyzer_root,
            "final_output_path": final_output_path,
        }
        work_items.append(job)

    # Display prepared jobs
    jobs = work_items[:]
    print(
        f"Prepared {len(jobs)} agent job(s):\n"
    )
    for i, job in enumerate(jobs, 1):
        print(f"{i:2d}. {job['name']:30s} {job['repo']}")
        print(f"    cwd: {job['cwd']}")
        print()

    # Create logs directory
    log_dir = Path(getattr(args, "log_dir", "logs/generate-architecture"))
    log_dir.mkdir(parents=True, exist_ok=True)
    print(f"Logs will be written to: {log_dir}\n")

    print(f"{'=' * 60}")
    print(f"Ready to process {len(work_items)} component(s)")
    print(f"Max concurrent agents: {args.max_concurrent}")
    print(f"Harness: {harness}")
    selected_model = args.model or (
        "opus" if harness == "claude" else "configured default"
    )
    print(f"Model: {selected_model}")
    print(f"{'=' * 60}\n")

    if jobs:
        await run_agents_concurrently(
            jobs,
            log_dir,
            args.model,
            args.max_concurrent,
            enable_skills=True,
            phase_label="PHASE 3 · Component architecture synthesis",
            harness=harness,
        )
