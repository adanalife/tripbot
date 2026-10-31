"""Synth-time checks on the PreSync image gate.

Unlike test_cdk8s.py these synth rather than read dist/: the gate renders only
for pinned envs, and prod-1's app manifests are frozen to their release version
(`task cdk8s:synth` discards the HEAD re-synth), so the committed dist only
grows a gate at the next version bump. Synthing is what shows it today.

Needs the cdk8s-generated `imports` package, which `task cdk8s:test` (and the
CI job) produce first; skipped when it's absent.
"""

import pytest

pytest.importorskip("imports.k8s")

import cdk8s  # noqa: E402

from adanalife_k8s.config import load_env  # noqa: E402
from adanalife_k8s.constructs.onscreens import OnscreensServer  # noqa: E402
from adanalife_k8s.constructs.tripbot import Tripbot  # noqa: E402

CASES = [("tripbot", Tripbot), ("onscreens", OnscreensServer)]


def _synth(ctor, env_name):
    chart = cdk8s.Testing.chart()
    ctor(chart, "twitch", env=load_env(env_name))
    return cdk8s.Testing.synth(chart)


def _gates(objs):
    return [o for o in objs if o["kind"] == "Job"]


@pytest.mark.parametrize("comp,ctor", CASES, ids=lambda v: getattr(v, "__name__", v))
def test_pinned_env_gates_on_its_own_image(comp, ctor):
    """The gate probes exactly the image its Deployment pulls — a gate on some
    other ref would pass a sync that still can't start."""
    objs = _synth(ctor, "prod-1")
    (gate,) = _gates(objs)
    assert gate["metadata"]["name"] == f"{comp}-twitch-image-gate"
    assert gate["metadata"]["annotations"]["argocd.argoproj.io/hook"] == "PreSync"

    deployment = next(o for o in objs if o["kind"] == "Deployment")
    app = next(
        c
        for c in deployment["spec"]["template"]["spec"]["containers"]
        if c["name"].startswith(comp)
    )
    probe = gate["spec"]["template"]["spec"]["containers"][0]
    assert probe["args"] == ["manifest", app["image"]]


@pytest.mark.parametrize("comp,ctor", CASES, ids=lambda v: getattr(v, "__name__", v))
def test_gate_pod_is_restricted_and_unselectable(comp, ctor):
    """PodSecurity `restricted` rejects a gate without runAsNonRoot, which would
    fail PreSync for a reason unrelated to the image. The pod must also stay out
    of the component Service's endpoints — it carries no `app` selector label."""
    (gate,) = _gates(_synth(ctor, "prod-1"))
    pod = gate["spec"]["template"]["spec"]
    assert pod["securityContext"]["runAsNonRoot"] is True
    assert pod["securityContext"]["runAsUser"] == 65532
    assert "app" not in gate["spec"]["template"]["metadata"]["labels"]


@pytest.mark.parametrize("comp,ctor", CASES, ids=lambda v: getattr(v, "__name__", v))
def test_floating_env_gets_no_gate(comp, ctor):
    """A floating tag always resolves to a prior build, so stage has nothing to
    gate on — and a gate there would run a registry probe on every sync."""
    assert _gates(_synth(ctor, "stage-1")) == []
