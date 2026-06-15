"""VlcServer — the dashcam video pipeline (libvlc + RTSP + HTTP API).

A per-platform factory (one instance per streaming platform, like ObsInstance):
`VlcServer(self, "twitch", env=env)` emits `vlc-twitch` objects, name resolved
through the contract via naming.app_name. Reproduces k8s/apps/vlc-server/base +
overlays:

  * RollingUpdate (readiness-gated; dashcam PVC is ReadOnlyMany so both pods can
    mount during the surge), seccomp + drop-ALL hardening, /health probes.
  * dashcam volume: NFS PVC (prod/stage, env.nfs_*) or hostPath (local/dev). The
    PVC is shared across platforms (ReadOnlyMany), so every platform's vlc pod
    mounts the same `vlc-dashcam` claim — it is NOT per-platform.
  * iGPU request on GPU envs; OTEL/Sentry envFrom from shared-secrets.
  * traefik Ingress (TLS on minipc) + optional Tailscale Ingress.
"""

from __future__ import annotations

from constructs import Construct

import imports.k8s as k8s
from adanalife_k8s import appconfig, configmap, scheduling
from adanalife_k8s.config import EnvConfig
from adanalife_k8s.naming import app_name, meta_labels, selector

MOUNT_PATH = "/opt/data/Dashcam/_all"

# Container ports (deployment.yaml order) vs Service ports (service.yaml order)
# — the legacy base lists them in different orders; reproduce both.
_CONTAINER_PORTS = [("http", 8080), ("vnc", 5900), ("rtsp", 8554)]
_SERVICE_PORTS = [("http", 8080), ("rtsp", 8554), ("vnc", 5900)]

# Constant base ConfigMap literals (kustomization configMapGenerator base set).
# VLC_SERVER_HOST is the pod's own name:port, so it's set per-instance in __init__.
_BASE_CONFIG = {
    "DISPLAY": ":0.0",
    "XDG_RUNTIME_DIR": "/root/.cache/xdgr",
    "FONTCONFIG_PATH": "/etc/fonts",
}


class VlcServer(Construct):
    def __init__(self, scope: Construct, platform: str, *, env: EnvConfig):
        name = app_name("vlc", platform)  # vlc-twitch / vlc-youtube
        super().__init__(scope, name)
        ns = env.namespace or None
        labels = meta_labels(name)
        sel = selector(name)

        container_ports = _CONTAINER_PORTS
        service_ports = _SERVICE_PORTS

        # --- ConfigMap (stable name + content-hash annotation) ---
        data = dict(_BASE_CONFIG)
        data["VLC_SERVER_HOST"] = f"{name}:8080"  # self-reference
        # OBS WebSocket control addr (:4455) — vlc-server polls OBS for streaming
        # state, so it must dial its OWN platform's OBS (vlc-youtube → obs-youtube),
        # not the baked-in obs-twitch default that left the YouTube vlc connecting
        # to obs-twitch. Set explicitly per platform (same as tripbot.py); relying
        # on the default already caused an incident (pkg/obs/control.go).
        data["OBS_WEBSOCKET_ADDR"] = f"{app_name('obs', platform)}:4455"
        if appconfig.uses_local_stubs(env):
            data.update(appconfig.local_stubs())
        data.update(appconfig.telemetry_config(env, platform))
        # vlc-server's own NATS command subscriber needs NATS_URL wherever
        # there's a platform NATS (dev/stage/prod), omitted on local — same
        # gate as onscreens.py.
        if env.nats_url:
            data["NATS_URL"] = env.nats_url
        # vlc-server scopes its JetStream lastplayed leaf (resume-on-restart)
        # by STREAM_PLATFORM. twitch is the binary's default, so — same idiom
        # as the tripbot/OBS charts — only non-twitch instances carry the key,
        # keeping the long-running twitch ConfigMaps (and their config-hash
        # rollouts) untouched.
        if platform != "twitch":
            data["STREAM_PLATFORM"] = platform
        cfg_hash = configmap.config_map(
            self,
            "config",
            name=f"{name}-config",
            namespace=ns,
            labels=labels,
            data=data,
        )

        # --- dashcam volume (nfs PVC | hostPath) ---
        # The NFS PV/PVC are NOT emitted here — they're stateful, so they live in
        # DataChart (emit_dashcam_volume), separate from this stateless Deployment
        # so app churn can't disturb them. This just references the PVC by name.
        # The claim is shared (ReadOnlyMany) across platforms — not per-platform.
        if env.dashcam_mode == "nfs":
            volume = k8s.Volume(
                name="dashcam",
                persistent_volume_claim=k8s.PersistentVolumeClaimVolumeSource(
                    claim_name="vlc-dashcam", read_only=True
                ),
            )
        else:
            volume = k8s.Volume(
                name="dashcam",
                host_path=k8s.HostPathVolumeSource(
                    path="/host/dashcam", type="Directory"
                ),
            )

        # --- resources (+ iGPU on GPU envs) ---
        # The CPU request is the CFS weight under contention — prod sizes it
        # for real (env.vlc_cpu_request) so co-tenant bursts can't starve the
        # decode chain (the 2026-06-11 prod-stutter incident).
        requests = {
            "cpu": k8s.Quantity.from_string(env.vlc_cpu_request),
            "memory": k8s.Quantity.from_string("512Mi"),
        }
        limits = {"memory": k8s.Quantity.from_string("2Gi")}
        # vlc does stream-copy + software decode and doesn't need the iGPU; the
        # claim is gated on vlc_gpu so an env can keep its GPU (for OBS) while
        # dropping just vlc's claim.
        if env.gpu and env.vlc_gpu:
            requests["gpu.intel.com/i915"] = k8s.Quantity.from_string("1")
            limits["gpu.intel.com/i915"] = k8s.Quantity.from_string("1")

        container = k8s.Container(
            name=name,
            image=f"adanalife/vlc:{env.tag_for('vlc')}",
            image_pull_policy=env.pull_policy_for("vlc"),
            security_context=k8s.SecurityContext(
                allow_privilege_escalation=False,
                capabilities=k8s.Capabilities(drop=["ALL"]),
            ),
            ports=[
                k8s.ContainerPort(name=n, container_port=p) for n, p in container_ports
            ],
            env_from=[
                k8s.EnvFromSource(
                    config_map_ref=k8s.ConfigMapEnvSource(name=f"{name}-config")
                ),
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
                initial_delay_seconds=15,
                period_seconds=30,
                timeout_seconds=5,
            ),
            readiness_probe=k8s.Probe(
                http_get=k8s.HttpGetAction(
                    path="/health/ready", port=k8s.IntOrString.from_string("http")
                ),
                initial_delay_seconds=5,
                period_seconds=10,
            ),
            resources=k8s.ResourceRequirements(requests=requests, limits=limits),
            volume_mounts=[
                k8s.VolumeMount(name="dashcam", mount_path=MOUNT_PATH, read_only=True)
            ],
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
                        # to the MS-01 when it's gone (stage only). The RTSP feed
                        # to OBS crosses the LAN instead of localhost when vlc
                        # lands on the Pi. See scheduling.py.
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
                        volumes=[volume],
                    ),
                ),
            ),
        )

        # --- Service ---
        svc_ports = [
            k8s.ServicePort(name=n, port=p, target_port=k8s.IntOrString.from_string(n))
            for n, p in service_ports
        ]
        k8s.KubeService(
            self,
            "service",
            metadata=k8s.ObjectMeta(name=name, namespace=ns, labels=labels),
            spec=k8s.ServiceSpec(type="ClusterIP", selector=sel, ports=svc_ports),
        )

        # --- host-access LoadBalancer (k3d-only convenience: local + dev, which
        # extends local). Overlay-added, so no metadata labels (matches render). ---
        if appconfig.uses_local_stubs(env):
            k8s.KubeService(
                self,
                "host-access",
                metadata=k8s.ObjectMeta(name=f"{name}-host", namespace=ns),
                spec=k8s.ServiceSpec(
                    type="LoadBalancer",
                    selector=sel,
                    ports=[
                        k8s.ServicePort(
                            name="vnc",
                            port=5903,
                            target_port=k8s.IntOrString.from_string("vnc"),
                        ),
                        k8s.ServicePort(
                            name="rtsp",
                            port=8554,
                            target_port=k8s.IntOrString.from_string("rtsp"),
                        ),
                    ],
                ),
            )

        # --- Ingress(es) — only where the env publishes DNS. Overlay-added, so
        # no metadata labels (the base `labels:` directive never touched them). ---
        if env.dns_base:
            self._ingress(name, env, ns)
        if env.tailscale and env.dns_base:
            self._tailscale_ingress(name, env, ns)

    # ---- helpers ----
    def _ingress(self, name, env: EnvConfig, ns):
        host = f"{name}.{env.dns_base}"
        ann = {"external-dns.alpha.kubernetes.io/hostname": host}
        if env.tls:
            ann["cert-manager.io/issuer"] = "letsencrypt-route53"
        backend = k8s.IngressBackend(
            service=k8s.IngressServiceBackend(
                name=name, port=k8s.ServiceBackendPort(name="http")
            )
        )
        k8s.KubeIngress(
            self,
            "ingress",
            metadata=k8s.ObjectMeta(name=name, namespace=ns, annotations=ann),
            spec=k8s.IngressSpec(
                ingress_class_name="traefik",
                tls=[k8s.IngressTls(hosts=[host], secret_name=f"{name}-tls")]
                if env.tls
                else None,
                rules=[
                    k8s.IngressRule(
                        host=host,
                        http=k8s.HttpIngressRuleValue(
                            paths=[
                                k8s.HttpIngressPath(
                                    path="/", path_type="Prefix", backend=backend
                                )
                            ]
                        ),
                    )
                ],
            ),
        )

    def _tailscale_ingress(self, name, env: EnvConfig, ns):
        short = env.dns_base.split(".")[0]  # prod / stage / dev
        k8s.KubeIngress(
            self,
            "ts-ingress",
            metadata=k8s.ObjectMeta(name=f"{name}-ts", namespace=ns),
            spec=k8s.IngressSpec(
                ingress_class_name="tailscale",
                default_backend=k8s.IngressBackend(
                    service=k8s.IngressServiceBackend(
                        name=name, port=k8s.ServiceBackendPort(number=8080)
                    )
                ),
                tls=[k8s.IngressTls(hosts=[f"{name}-{short}"])],
            ),
        )


# NB: the dashcam PV/PVC emitters (emit_dashcam_pv / emit_dashcam_pvc) are NOT
# here — they're stateful, data-layer objects that stay in infra/cdk8s (DataChart
# + DashcamPVChart). VlcServer above only *references* the `vlc-dashcam` PVC by
# name; whoever owns the cluster's data unit (infra) provisions the claim + PV.
