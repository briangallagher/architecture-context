from pathlib import Path

SKILL_PATH = (
    Path(__file__).resolve().parent.parent
    / ".claude/skills/repo-to-architecture-summary/SKILL.md"
)


def test_patch_output_contract_is_explicit_in_summary_skill():
    skill = SKILL_PATH.read_text()

    assert "--patch-output=FILENAME" in skill
    assert '"schema_version": 1' in skill
    assert '"operations": [' in skill
    assert '"action": "add"' in skill
    assert '"analyzer_value": null' in skill
    assert "repository-relative path followed by a numeric line" in skill
    assert "bare paths, directory paths, glob patterns" in skill
    assert "Do not add prose or Markdown around" in skill
    assert "never `metadata`" in skill
    assert "`[endpoint, methods]`" in skill
    assert "`[component, interaction_type]`" in skill
    assert "at most one operation" in skill
    assert '["Tracking Server API", "All"]' in skill
    assert "mechanism is a cell value" in skill
    assert "one bounded search plan" in skill
    assert "Do not repeat equivalent searches" in skill
    assert "stop discovery for that gap" in skill
    assert "not an invitation to continue searching" in skill
    assert "not permission to stop early" not in skill
    assert "not permission to repeat" in skill
    assert '"category": "architecture_components"' in skill
    assert "do not copy the candidate row into `candidate_value`" in skill
    assert "Never use `update` to\nchange a key column" in skill
    assert (
        "requires a delete for the former key and an add\nfor the latter key"
        in skill
    )
    assert "Always write the artifact" in skill


def test_surface_planning_and_final_review_contract_is_explicit():
    skill = SKILL_PATH.read_text()

    assert "--surface-inventory=PATH" in skill
    assert "--surface-coverage-output=PATH" in skill
    assert "plan\nacross individual surfaces" in skill
    assert "Closing one gateway or authentication" in skill
    assert "one justified targeted follow-up" in skill
    assert "final evidence-to-output review" in skill
    assert "without another read" in skill
    assert "post-merge validation" in skill
