"""Phase 5: Generate deterministic version navigation indexes without an agent."""

from pathlib import Path

from lib.fetch import _ensure_arch_analyzer
from lib.structured_component_publication import has_publication_state
from lib.structured_component_synthesis import GoStructuredAssembler
from lib.version_index import generate_version_index


async def run_generate_index_phase(args) -> None:
    """Run Phase 5 for one version using only local architecture artifacts."""

    print("\n" + "=" * 60)
    print("PHASE 5: Rendering deterministic version navigation")
    print("=" * 60 + "\n")

    architecture_dir = Path(args.architecture_dir)
    platform_dir = architecture_dir / args.platform
    assembler = None
    if has_publication_state(platform_dir):
        assembler = GoStructuredAssembler(
            (await _ensure_arch_analyzer(),),
            Path(__file__).resolve().parents[2] / "src/arch-analyzer",
        )
    result = generate_version_index(
        platform_dir,
        platforms_file=Path(getattr(args, "platforms_file", "platforms.yaml")),
        overlays_dir=Path(getattr(args, "overlays_dir", "overlays")),
        assembler=assembler,
    )

    action = "updated" if result.changed else "unchanged"
    print(f"{result.path}: {action}; {result.component_count} inventory component(s)")
