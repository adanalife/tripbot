"""Synth-time checks on the committed deploy units in cdk8s/dist/.

Reads the committed YAML rather than re-synthing — the cdk8s dependency (jsii
needs node) stays out of the default test run; the cdk8s-synth CI job covers
dist/ freshness separately (golden gate).
"""

from pathlib import Path

import pytest
import yaml

DIST = Path(__file__).resolve().parents[1] / "dist"
VERSIONS = Path(__file__).resolve().parents[1] / "versions.yaml"

# (env, platform) the app charts are synthed for. Mirrors config.ENVS platforms.
ENV_PLATFORMS = {
    "prod-1": ("twitch",),
    "stage-1": ("youtube", "twitch"),
    "development": ("twitch",),
    "local": ("twitch",),
}
COMPONENTS = ("tripbot", "onscreens")


def _objects(stem: str) -> list[dict]:
    with (DIST / f"{stem}.k8s.yaml").open() as f:
        return [o for o in yaml.safe_load_all(f) if o]


def _by_kind(objs: list[dict], kind: str) -> list[dict]:
    return [o for o in objs if o["kind"] == kind]


def _env_components():
    for env, platforms in ENV_PLATFORMS.items():
        for platform in platforms:
            for comp in COMPONENTS:
                yield env, comp, platform


@pytest.mark.parametrize(
    "env,comp,platform", list(_env_components()), ids=lambda v: str(v)
)
def test_each_component_has_deployment_and_service(env, comp, platform):
    objs = _objects(f"{env}-{comp}-{platform}")
    kinds = {o["kind"] for o in objs}
    assert "Deployment" in kinds, f"{env}-{comp}-{platform} missing Deployment"
    assert "Service" in kinds, f"{env}-{comp}-{platform} missing Service"


@pytest.mark.parametrize("env,platform", [("prod-1", "twitch"), ("stage-1", "youtube")])
@pytest.mark.parametrize("comp", ["tripbot"])
def test_obs_websocket_addr_is_platform_scoped(env, comp, platform):
    """tripbot's OBS-websocket client (watchdog + stream start/stop) must dial
    its OWN platform's OBS — the youtube instance dials obs-youtube, not the
    baked-in obs-twitch default."""
    cms = _by_kind(_objects(f"{env}-{comp}-{platform}"), "ConfigMap")
    data = next(cm["data"] for cm in cms if "OBS_WEBSOCKET_ADDR" in cm.get("data", {}))
    assert data["OBS_WEBSOCKET_ADDR"] == f"obs-{platform}:4455"


@pytest.mark.parametrize("env,platform", [("prod-1", "twitch"), ("stage-1", "youtube")])
def test_prod_pinned_stage_floats(env, platform):
    """prod deploys the exact versions.yaml pin with IfNotPresent; stage floats
    on main with Always."""
    pins = yaml.safe_load(VERSIONS.read_text())
    dep = _by_kind(_objects(f"{env}-tripbot-{platform}"), "Deployment")[0]
    container = next(
        c
        for c in dep["spec"]["template"]["spec"]["containers"]
        if c["name"].startswith("tripbot")
    )
    # Split on the first colon: the repo has no registry-host:port prefix, and
    # the tag itself may carry a @sha256 digest suffix (a digest-pinned deploy).
    image, tag = container["image"].split(":", 1)
    assert image == "adanalife/tripbot"  # public Docker Hub image, no v-prefix tag
    if env == "prod-1":
        assert tag == pins["prod-1"]["tripbot"]
        assert container["imagePullPolicy"] == "IfNotPresent"
    else:
        assert tag == "main"
        assert container["imagePullPolicy"] == "Always"


@pytest.mark.parametrize("env", list(ENV_PLATFORMS))
def test_identity_unit_emits_app_secrets(env):
    objs = _objects(f"{env}-tripbot-identity")
    es_names = {o["metadata"]["name"] for o in objs if o["kind"] == "ExternalSecret"}
    secret_names = {o["metadata"]["name"] for o in objs if o["kind"] == "Secret"}
    # twitch + maps are required app creds in every env.
    assert {"tripbot-twitch-creds", "tripbot-google-maps-api-key"} <= es_names
    if env == "local":
        # laptop carries on-disk DB creds, not an ESO ExternalSecret.
        assert "tripbot-secret" in secret_names
    else:
        assert "tripbot-database-creds" in es_names


def test_prod_stream_protection_priorityclass_only_in_prod():
    prod = {o["kind"] for o in _objects("prod-1-tripbot-identity")}
    assert "PriorityClass" in prod
    # stage carries the ResourceQuota but no prod PriorityClass.
    stage = _objects("stage-1-tripbot-identity")
    assert "PriorityClass" not in {o["kind"] for o in stage}
    assert "ResourceQuota" in {o["kind"] for o in stage}


def test_youtube_tripbot_emits_youtube_creds():
    objs = _objects("stage-1-tripbot-youtube")
    es_names = {o["metadata"]["name"] for o in objs if o["kind"] == "ExternalSecret"}
    assert "tripbot-youtube-creds" in es_names


def test_stage_parks_every_platform():
    """Every stage platform Deployment renders replicas:0 — the resting state is
    everything-off; a platform comes online via the console's mode switch. Prod's
    replica count isn't pinned here: replicas are runtime-owned (Argo ignores
    .spec.replicas per infra#877), so prod births at 0 too and a live scale
    sticks — the committed prod value is just whatever the last release synthed
    (0), not a policy this test guards."""

    def _deploy(stem):
        return _by_kind(_objects(stem), "Deployment")[0]

    for platform in ("twitch", "youtube", "tiktok", "facebook", "instagram"):
        stem = f"stage-1-tripbot-{platform}"
        assert _deploy(stem)["spec"]["replicas"] == 0, f"{stem} should be parked"
    # prod still renders its Deployment (existence is the invariant; the replica
    # count is Argo-ignored, so it isn't asserted).
    assert _deploy("prod-1-tripbot-twitch")


def test_stage_twitch_routes_through_gateway():
    """Both stage and prod tripbot-twitch carry TWITCH_API_URL (the gateway is
    the single Helix caller since the cutover); the youtube instances do not."""

    def _cm_data(stem):
        return _by_kind(_objects(stem), "ConfigMap")[0]["data"]

    assert (
        _cm_data("stage-1-tripbot-twitch").get("TWITCH_API_URL")
        == "http://gateway-twitch.stage-1.svc.cluster.local:8080"
    )
    assert (
        _cm_data("prod-1-tripbot-twitch").get("TWITCH_API_URL")
        == "http://gateway-twitch.prod-1.svc.cluster.local:8080"
    )
    assert "TWITCH_API_URL" not in _cm_data("stage-1-tripbot-youtube")
    assert "TWITCH_API_URL" not in _cm_data("prod-1-tripbot-youtube")


def test_stage_youtube_routes_sends_through_gateway():
    """Stage tripbot-youtube carries YOUTUBE_API_URL (outbound send via the
    gateway); the twitch instance does not."""

    def _cm_data(stem):
        return _by_kind(_objects(stem), "ConfigMap")[0]["data"]

    assert (
        _cm_data("stage-1-tripbot-youtube").get("YOUTUBE_API_URL")
        == "http://gateway-youtube.stage-1.svc.cluster.local:8080"
    )
    assert "YOUTUBE_API_URL" not in _cm_data("stage-1-tripbot-twitch")


def _pod_spec(stem: str) -> dict:
    return _by_kind(_objects(stem), "Deployment")[0]["spec"]["template"]["spec"]


def _prefers_rpi5(spec: dict) -> bool:
    """True iff the pod tolerates the rpi5 taint AND prefers the board label."""
    tolerates = any(
        t.get("key") == "dana.lol/rpi5" for t in spec.get("tolerations", [])
    )
    prefs = (
        spec.get("affinity", {})
        .get("nodeAffinity", {})
        .get("preferredDuringSchedulingIgnoredDuringExecution", [])
    )
    biases = any(
        req.get("key") == "dana.lol/board" and "rpi5" in req.get("values", [])
        for term in prefs
        for req in term.get("preference", {}).get("matchExpressions", [])
    )
    return tolerates and biases


def _colocates_with_obs(spec: dict, obs_app: str) -> bool:
    """True iff the pod prefers (podAffinity) the node running `obs_app`."""
    prefs = (
        spec.get("affinity", {})
        .get("podAffinity", {})
        .get("preferredDuringSchedulingIgnoredDuringExecution", [])
    )
    return any(
        term.get("podAffinityTerm", {}).get("topologyKey") == "kubernetes.io/hostname"
        and term.get("podAffinityTerm", {})
        .get("labelSelector", {})
        .get("matchLabels", {})
        .get("app")
        == obs_app
        for term in prefs
    )


def test_stage_tripbot_prefers_rpi5():
    """tripbot-youtube is control-plane (chat/EventSub), not a realtime OBS feeder,
    so it keeps the independent rpi5 node-preference — tolerate the taint and bias
    toward the board label, recovering onto the MS-01 when the Pi is gone."""
    assert _prefers_rpi5(_pod_spec("stage-1-tripbot-youtube"))


def test_stage_obs_feeders_colocate_with_obs():
    """onscreens feeds OBS continuously (browser-source) and must reach it on
    localhost, not across the LAN. It anchors to its platform's OBS pod via
    podAffinity instead of pulling toward the Pi on its own — keeping the
    rpi5 toleration so it can follow OBS onto the Pi, but NOT an independent
    board node-affinity, which splits the pipeline across nodes (and stutters
    the stream) whenever OBS spills to the MS-01."""
    spec = _pod_spec("stage-1-onscreens-youtube")
    assert _colocates_with_obs(spec, "obs-youtube")
    # follows OBS onto the Pi if OBS lands there ...
    assert any(t.get("key") == "dana.lol/rpi5" for t in spec.get("tolerations", []))
    # ... but carries no independent rpi5 board pull.
    assert not _prefers_rpi5(spec)
