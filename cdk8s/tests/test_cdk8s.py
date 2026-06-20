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
COMPONENTS = ("tripbot", "vlc", "onscreens", "obs")


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


def test_prod_vlc_rtsp_nodeport():
    """prod vlc-twitch emits a fixed RTSP NodePort so a LAN box (OBS on a desktop)
    can pull rtsp://<node-ip>:30854/dashcam without kubectl. The minipc has no
    LoadBalancer controller, so it's a NodePort not an LB. Pinned at 30854."""
    svcs = _by_kind(_objects("prod-1-vlc-twitch"), "Service")
    np = next((s for s in svcs if s["metadata"]["name"] == "vlc-twitch-rtsp"), None)
    assert np is not None, "prod vlc-twitch missing the vlc-twitch-rtsp NodePort"
    assert np["spec"]["type"] == "NodePort"
    [port] = np["spec"]["ports"]
    assert port["nodePort"] == 30854
    assert port["port"] == 8554 and port["targetPort"] == "rtsp"


@pytest.mark.parametrize("stem", ["stage-1-vlc-youtube", "development-vlc-twitch"])
def test_non_prod_has_no_rtsp_nodeport(stem):
    """The RTSP NodePort is prod-only — stage/dev don't set vlc_rtsp_node_port (a
    pinned NodePort can't be claimed twice on the co-tenant minipc node)."""
    svcs = _by_kind(_objects(stem), "Service")
    assert not any(s["spec"].get("type") == "NodePort" for s in svcs)


@pytest.mark.parametrize("env,platform", [("prod-1", "twitch"), ("stage-1", "youtube")])
@pytest.mark.parametrize("comp", ["vlc", "tripbot"])
def test_obs_websocket_addr_is_platform_scoped(env, comp, platform):
    """Both OBS-websocket clients (tripbot + vlc-server poll/control OBS) must dial
    their OWN platform's OBS — vlc-youtube → obs-youtube, not the baked-in
    obs-twitch default that broke the YouTube vlc."""
    cms = _by_kind(_objects(f"{env}-{comp}-{platform}"), "ConfigMap")
    data = next(cm["data"] for cm in cms if "OBS_WEBSOCKET_ADDR" in cm.get("data", {}))
    assert data["OBS_WEBSOCKET_ADDR"] == f"obs-{platform}:4455"


@pytest.mark.parametrize("env,platform", [("prod-1", "twitch"), ("stage-1", "youtube")])
def test_prod_pinned_stage_floats(env, platform):
    """prod deploys the exact versions.yaml pin with IfNotPresent; stage floats
    on develop with Always."""
    pins = yaml.safe_load(VERSIONS.read_text())
    dep = _by_kind(_objects(f"{env}-tripbot-{platform}"), "Deployment")[0]
    container = next(
        c
        for c in dep["spec"]["template"]["spec"]["containers"]
        if c["name"].startswith("tripbot")
    )
    image, tag = container["image"].rsplit(":", 1)
    assert image == "adanalife/tripbot"  # public Docker Hub image, no v-prefix tag
    if env == "prod-1":
        assert tag == pins["prod-1"]["tripbot"]
        assert container["imagePullPolicy"] == "IfNotPresent"
    else:
        assert tag == "develop"
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


def test_stage_omits_replicas_prod_keeps_one():
    """Stage app Deployments omit spec.replicas so a hand/console scale is
    authoritative (manual_replicas); prod keeps replicas:1 so Argo holds it."""

    def _deploy(stem):
        return _by_kind(_objects(stem), "Deployment")[0]

    for stem in ("stage-1-tripbot-twitch", "stage-1-tripbot-youtube"):
        assert "replicas" not in _deploy(stem)["spec"], f"{stem} should omit replicas"
    assert _deploy("prod-1-tripbot-twitch")["spec"]["replicas"] == 1


def test_stage_twitch_routes_through_gateway():
    """Stage tripbot-twitch carries TWITCH_API_URL (Phase 3 gateway); the youtube
    instance and prod tripbot do not."""

    def _cm_data(stem):
        return _by_kind(_objects(stem), "ConfigMap")[0]["data"]

    assert (
        _cm_data("stage-1-tripbot-twitch").get("TWITCH_API_URL")
        == "http://gateway-twitch.stage-1.svc.cluster.local:8080"
    )
    assert "TWITCH_API_URL" not in _cm_data("stage-1-tripbot-youtube")
    assert "TWITCH_API_URL" not in _cm_data("prod-1-tripbot-twitch")


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


# Which (env, platform) OBS instances actually stream — must mirror
# config.EnvConfig.obs_streaming. A streaming instance emits its stream-key
# ExternalSecret; an idle one does not (and boots without the key).
STREAMING = {("prod-1", "twitch"), ("stage-1", "youtube")}


@pytest.mark.parametrize("env,comp,platform", list(_env_components()), ids=str)
def test_obs_stream_key_secret_iff_streaming(env, comp, platform):
    if comp != "obs":
        pytest.skip("stream-key toggle is OBS-only")
    objs = _objects(f"{env}-obs-{platform}")
    es_names = {o["metadata"]["name"] for o in _by_kind(objs, "ExternalSecret")}
    # twitch keeps the shared base name; other platforms get a distinct one.
    key_name = (
        "obs-stream-key" if platform == "twitch" else f"obs-{platform}-stream-key"
    )
    if (env, platform) in STREAMING:
        assert key_name in es_names, (
            f"{env}/{platform} should stream but has no stream-key ExternalSecret"
        )
    else:
        assert key_name not in es_names, (
            f"{env}/{platform} should be idle but emits a stream-key ExternalSecret"
        )


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
    """vlc + onscreens feed OBS continuously (RTSP / browser-source) and must reach
    it on localhost, not across the LAN. They anchor to their platform's OBS pod
    via podAffinity instead of pulling toward the Pi on their own — keeping the
    rpi5 toleration so they can follow OBS onto the Pi, but NOT the board
    node-affinity that previously split the pipeline when OBS spilled to the MS-01
    (the 2026-06-19 stage obs-youtube stutter)."""
    for stem in ("stage-1-vlc-youtube", "stage-1-onscreens-youtube"):
        spec = _pod_spec(stem)
        assert _colocates_with_obs(spec, "obs-youtube"), stem
        # follows OBS onto the Pi if OBS lands there ...
        assert any(
            t.get("key") == "dana.lol/rpi5" for t in spec.get("tolerations", [])
        ), stem
        # ... but no longer carries an independent rpi5 board pull.
        assert not _prefers_rpi5(spec), stem


def test_stage_vaapi_obs_stays_on_msi():
    """Stage obs-youtube is a VAAPI encoder (holds the i915 claim), so it must
    NOT bias toward the Pi (no H.264 hw encoder there) — the resource claim
    hard-gates it onto the MS-01 and the rpi5 affinity drops out together."""
    assert not _prefers_rpi5(_pod_spec("stage-1-obs-youtube"))


def test_prod_vaapi_obs_stays_on_msi():
    """Prod obs-twitch is a VAAPI encoder (holds the i915 claim), so it must NOT
    bias toward the Pi (no H.264 hw encoder there) — it stays on the MS-01."""
    assert not _prefers_rpi5(_pod_spec("prod-1-obs-twitch"))
