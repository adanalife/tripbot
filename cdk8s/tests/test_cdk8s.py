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
    "stage-1": ("youtube",),
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
