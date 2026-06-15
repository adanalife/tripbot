"""OnscreensServer — the standalone onscreens HTTP API + NATS middle-text subscriber.

Reproduces k8s/apps/onscreens-server/base + overlays. Formerly a second port on
the vlc-server pod (:8081); now its own stateless Deployment on :8080 that

  * serves the onscreens feeds OBS browser sources poll (ONSCREENS_URL_BASE) and
    the HTTP API tripbot calls (ONSCREENS_SERVER_HOST),
  * subscribes to NATS `tripbot.<env>.onscreens.middle.show` — NATS_URL lands in
    its config on dev/stage/prod; absent on local (the subscriber is a graceful
    no-op when NATS_URL is unset).

Config surface is small: just the shared telemetry block + NATS_URL. No dashcam
volume, no GPU, no stubs (the binary's config only requires ENV), no Ingress
(cluster-internal only — tripbot + OBS reach it by Service DNS). RollingUpdate
maxSurge=1/maxUnavailable=0: state is in-memory and rebuilt on boot, so a brief
overlap is harmless and a failed rollout leaves the old pod serving.
"""

from __future__ import annotations

from constructs import Construct

import imports.k8s as k8s
from adanalife_k8s import appconfig, configmap, scheduling
from adanalife_k8s.config import EnvConfig
from adanalife_k8s.naming import app_name, meta_labels, selector

RUN_DIR = "/opt/data/run"


class OnscreensServer(Construct):
    def __init__(self, scope: Construct, platform: str, *, env: EnvConfig):
        name = app_name("onscreens", platform)  # onscreens-twitch / onscreens-youtube
        super().__init__(scope, name)
        ns = env.namespace or None
        labels = meta_labels(name)
        sel = selector(name)

        # --- ConfigMap (stable name + content-hash annotation) ---
        # Telemetry block (ENV + OTEL_* + SENTRY_ENVIRONMENT) is shared with the
        # other Go services; NATS_URL is present wherever there's a platform NATS
        # (dev/stage/prod) and omitted on local.
        data = dict(appconfig.telemetry_config(env, platform))
        if env.nats_url:
            data["NATS_URL"] = env.nats_url
        # Per-platform rotator-message filtering: a YouTube overlay must not
        # advertise Twitch-only commands (!miles, !guess). onscreens-server reads
        # ONSCREENS_SERVER_PLATFORM (defaults to twitch if unset).
        data["ONSCREENS_SERVER_PLATFORM"] = platform
        cfg_hash = configmap.config_map(
            self,
            "config",
            name=f"{name}-config",
            namespace=ns,
            labels=labels,
            data=data,
        )

        container = k8s.Container(
            name=name,
            image=f"adanalife/onscreens-server:{env.tag_for('onscreens-server')}",
            image_pull_policy=env.pull_policy_for("onscreens-server"),
            security_context=k8s.SecurityContext(
                allow_privilege_escalation=False,
                capabilities=k8s.Capabilities(drop=["ALL"]),
            ),
            ports=[k8s.ContainerPort(name="http", container_port=8080)],
            env_from=[
                k8s.EnvFromSource(
                    config_map_ref=k8s.ConfigMapEnvSource(name=f"{name}-config")
                ),
                # onscreens-server reuses the vlc-server Sentry DSN for now.
                k8s.EnvFromSource(
                    secret_ref=k8s.SecretEnvSource(
                        name="sentry-vlc-server", optional=False
                    )
                ),
                k8s.EnvFromSource(
                    secret_ref=k8s.SecretEnvSource(
                        name="grafana-cloud-otlp", optional=False
                    )
                ),
            ],
            liveness_probe=k8s.Probe(
                http_get=k8s.HttpGetAction(
                    path="/health/live", port=k8s.IntOrString.from_string("http")
                ),
                initial_delay_seconds=5,
                period_seconds=30,
                timeout_seconds=5,
            ),
            readiness_probe=k8s.Probe(
                http_get=k8s.HttpGetAction(
                    path="/health/ready", port=k8s.IntOrString.from_string("http")
                ),
                initial_delay_seconds=3,
                period_seconds=10,
            ),
            resources=k8s.ResourceRequirements(
                requests={
                    "cpu": k8s.Quantity.from_string("25m"),
                    "memory": k8s.Quantity.from_string("32Mi"),
                },
                limits={"memory": k8s.Quantity.from_string("128Mi")},
            ),
            # Writable tmpfs scratch for the RUN_DIR pidfile — nothing durable.
            volume_mounts=[k8s.VolumeMount(name="run", mount_path=RUN_DIR)],
        )

        k8s.KubeDeployment(
            self,
            "deployment",
            metadata=k8s.ObjectMeta(name=name, namespace=ns, labels=labels),
            spec=k8s.DeploymentSpec(
                replicas=1,
                strategy=k8s.DeploymentStrategy(
                    type="RollingUpdate",
                    rolling_update=k8s.RollingUpdateDeployment(
                        max_surge=k8s.IntOrString.from_number(1),
                        max_unavailable=k8s.IntOrString.from_number(0),
                    ),
                ),
                selector=k8s.LabelSelector(match_labels=sel),
                template=k8s.PodTemplateSpec(
                    metadata=k8s.ObjectMeta(
                        labels=sel, annotations=configmap.pod_annotations(cfg_hash)
                    ),
                    spec=k8s.PodSpec(
                        security_context=k8s.PodSecurityContext(
                            seccomp_profile=k8s.SeccompProfile(type="RuntimeDefault")
                        ),
                        priority_class_name=env.priority_class or None,
                        # Prefer the ephemeral rpi5 worker when present, recover
                        # to the MS-01 when it's gone (stage only). See scheduling.py.
                        tolerations=(
                            scheduling.prefer_rpi5_tolerations()
                            if env.prefer_rpi5
                            else None
                        ),
                        affinity=(
                            scheduling.prefer_rpi5_affinity()
                            if env.prefer_rpi5
                            else None
                        ),
                        containers=[container],
                        volumes=[
                            k8s.Volume(name="run", empty_dir=k8s.EmptyDirVolumeSource())
                        ],
                    ),
                ),
            ),
        )

        # --- Service (cluster-internal; tripbot + OBS reach :8080 by DNS) ---
        k8s.KubeService(
            self,
            "service",
            metadata=k8s.ObjectMeta(name=name, namespace=ns, labels=labels),
            spec=k8s.ServiceSpec(
                type="ClusterIP",
                selector=sel,
                ports=[
                    k8s.ServicePort(
                        name="http",
                        port=8080,
                        target_port=k8s.IntOrString.from_string("http"),
                    )
                ],
            ),
        )
