"""Resolve the repository's shared agent skills."""

from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
SHARED_SKILLS_DIR = REPO_ROOT / ".claude" / "skills"


def resolve_skill_file(name: str) -> Path:
    """Return the checked-in SKILL.md for an agent skill."""
    if not name or any(part in {"", ".", ".."} for part in Path(name).parts):
        raise ValueError(f"Invalid skill name: {name!r}")
    skill_file = (SHARED_SKILLS_DIR / name / "SKILL.md").resolve()
    shared_root = SHARED_SKILLS_DIR.resolve()
    if shared_root not in skill_file.parents or not skill_file.is_file():
        raise ValueError(
            f"Unknown skill {name!r}; expected {skill_file}"
        )
    return skill_file
