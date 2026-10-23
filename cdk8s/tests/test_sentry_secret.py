"""The Sentry Secret is required in prod and optional everywhere else.

Synths rather than reading dist/: prod-1's app manifests are frozen to their
release version, so the committed prod dist only changes at the next bump.
"""

import pytest

pytest.importorskip("imports.k8s")

import cdk8s  # noqa: E402

from adanalife_k8s.config import load_env  # noqa: E402
from adanalife_k8s.constructs.onscreens import OnscreensServer  # noqa: E402
from adanalife_k8s.constructs.tripbot import Tripbot  # noqa: E402

CASES = [("sentry-tripbot", Tripbot), ("sentry-onscreens-server", OnscreensServer)]


def _sentry_optional(ctor, secret, env_name):
    chart = cdk8s.Testing.chart()
    ctor(chart, "twitch", env=load_env(env_name))
    (dep,) = [o for o in cdk8s.Testing.synth(chart) if o["kind"] == "Deployment"]
    (ref,) = [
        src["secretRef"]
        for c in dep["spec"]["template"]["spec"]["containers"]
        for src in c.get("envFrom", [])
        if src.get("secretRef", {}).get("name") == secret
    ]
    return ref.get("optional", False)


@pytest.mark.parametrize("secret,ctor", CASES, ids=[s for s, _ in CASES])
@pytest.mark.parametrize("env_name,optional", [("prod-1", False), ("stage-1", True)])
def test_sentry_secret_required_only_in_prod(secret, ctor, env_name, optional):
    """A prod pod whose Sentry ExternalSecret never synced must fail to start
    rather than run with error reporting silently off."""
    assert _sentry_optional(ctor, secret, env_name) is optional
