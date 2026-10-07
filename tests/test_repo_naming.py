from lib.repo_naming import checkout_name, extra_repo_checkout_name


def test_checkout_name_applies_prefix_once():
    assert checkout_name("grid", "praxis-") == "praxis-grid"
    assert checkout_name("praxis-grid", "praxis-") == "praxis-grid"
    assert checkout_name("grid", "") == "grid"


def test_extra_repo_checkout_name_uses_configured_prefix():
    config = {
        "extra_repos": [
            {"org": "praxis-proxy", "repo": "grid", "name_prefix": "praxis-"},
        ],
    }
    assert (
        extra_repo_checkout_name(config, "praxis-proxy", "grid")
        == "praxis-grid"
    )
    assert extra_repo_checkout_name(config, "other", "grid") == "grid"
