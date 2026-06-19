"""ObsInstance — one OBS deployment for a single streaming platform.

Replaces the Kustomize obs/base + nameSuffix(-twitch/-youtube) + label-patch
idiom with a first-class factory. `ObsInstance(platform="youtube", ...)` emits
cleanly-named `obs-youtube` objects (no `obs-twitch-youtube` double-suffix), with
`app: obs-youtube` selector derived from the instance name, so a Service can only
ever select its own pods. Faithfully reproduces k8s/apps/obs/base + the per-env
overlays (GPU, encoder, quality, stream-key toggle, ingress).
"""

from __future__ import annotations

import hashlib
import json

from constructs import Construct

import imports.k8s as k8s
from adanalife_k8s.config import EnvConfig
from adanalife_k8s.contract import load_contract
from adanalife_k8s.naming import app_name
from adanalife_k8s.scheduling import prefer_rpi5_affinity, prefer_rpi5_tolerations

# (port-name, port-number) — order/names match k8s/apps/obs/base/{deployment,service}.yaml
_PORTS = [
    ("vnc", "obs_vnc"),
    ("websocket", "obs_websocket"),
    ("novnc", "obs_novnc"),
    ("obs-server", "obs_server"),
]


class ObsInstance(Construct):
    def __init__(
        self,
        scope: Construct,
        platform: str,  # "twitch" | "youtube"
        *,
        env: EnvConfig,
        streaming: bool = False,  # create the stream-key ExternalSecret
        stream_key_sm: str | None = None,  # SM path, e.g. k8s/obs/twitch-stream-key
        extra_config: dict[str, str] | None = None,
    ):
        c = load_contract()
        name = app_name("obs", platform)  # obs-twitch / obs-youtube
        super().__init__(scope, name)

        labels = {
            "app": name,
            "app.kubernetes.io/name": "obs",
            "app.kubernetes.io/instance": name,
            "app.kubernetes.io/part-of": "tripbot",
        }
        ns = env.namespace or None

        # --- ConfigMap (obs-config-<platform>) ---
        data = {
            "DASHCAM_RTSP_URL": c.dashcam_rtsp_url(platform),
            "ONSCREENS_URL_BASE": c.onscreens_url_base(platform),
            "VLC_URL_BASE": c.vlc_url_base(platform),
            "OBS_WEBSOCKET_PASSWD": "adanalife",
            "OBS_QUALITY_PRESET": env.obs_quality,
            "OBS_STREAM_ENCODER": env.obs_encoder,
            **(extra_config or {}),
        }
        cm_name = f"{name}-config"
        k8s.KubeConfigMap(
            self,
            "config",
            metadata=k8s.ObjectMeta(name=cm_name, namespace=ns, labels=labels),
            data=data,
        )
        cfg_hash = hashlib.sha256(
            json.dumps(data, sort_keys=True).encode()
        ).hexdigest()[:10]

        # --- stream-key Secret/ExternalSecret (streaming toggle) ---
        # twitch keeps the shared base name `obs-stream-key`; youtube gets a
        # distinct name so a twitch stream:on can't leak its key into youtube.
        secret_name = "obs-stream-key" if platform == "twitch" else f"{name}-stream-key"
        if streaming and stream_key_sm:
            self._external_secret(secret_name, stream_key_sm, ns, labels)

        env_from = [
            k8s.EnvFromSource(config_map_ref=k8s.ConfigMapEnvSource(name=cm_name)),
            # optional so the pod boots idle (VNC-only) when the Secret is absent.
            k8s.EnvFromSource(
                secret_ref=k8s.SecretEnvSource(name=secret_name, optional=True)
            ),
        ]

        # --- resources (+ iGPU on GPU envs) ---
        # The CPU request is the CFS weight under contention — prod sizes it
        # for real (env.obs_cpu_request) so co-tenant bursts can't starve the
        # encoder (the 2026-06-11 prod-stutter incident).
        requests = {
            "cpu": k8s.Quantity.from_string(env.obs_cpu_request),
            "memory": k8s.Quantity.from_string("512Mi"),
        }
        limits = {"memory": k8s.Quantity.from_string("3Gi")}
        # iGPU claim gated on (gpu and obs_gpu) — an env can drop just OBS's claim
        # (env.obs_gpu=False) to stop being a live VAAPI consumer on the shared
        # iGPU while still streaming via software x264 (env.obs_encoder).
        obs_uses_gpu = env.gpu and env.obs_gpu
        if obs_uses_gpu:
            requests["gpu.intel.com/i915"] = k8s.Quantity.from_string("1")
            limits["gpu.intel.com/i915"] = k8s.Quantity.from_string("1")

        container = k8s.Container(
            name="obs",
            image=f"adanalife/obs:{env.tag_for('obs')}",
            image_pull_policy=env.pull_policy_for("obs"),
            security_context=k8s.SecurityContext(
                allow_privilege_escalation=False,
                capabilities=k8s.Capabilities(drop=["ALL"]),
            ),
            ports=[
                k8s.ContainerPort(name=n, container_port=c.port(p)) for n, p in _PORTS
            ],
            env_from=env_from,
            liveness_probe=k8s.Probe(
                exec=k8s.ExecAction(command=["/opt/obs/healthcheck.sh"]),
                initial_delay_seconds=15,
                period_seconds=30,
                timeout_seconds=10,
                failure_threshold=3,
            ),
            resources=k8s.ResourceRequirements(requests=requests, limits=limits),
        )

        k8s.KubeDeployment(
            self,
            "deployment",
            metadata=k8s.ObjectMeta(name=name, namespace=ns, labels=labels),
            spec=k8s.DeploymentSpec(
                replicas=env.replicas,
                # Recreate: one Xvfb/VNC owner, no overlapping handoff.
                strategy=k8s.DeploymentStrategy(type="Recreate"),
                selector=k8s.LabelSelector(match_labels={"app": name}),
                template=k8s.PodTemplateSpec(
                    metadata=k8s.ObjectMeta(
                        labels=labels,
                        annotations={"adanalife.dev/config-hash": cfg_hash},
                    ),
                    spec=k8s.PodSpec(
                        security_context=k8s.PodSecurityContext(
                            seccomp_profile=k8s.SeccompProfile(type="RuntimeDefault")
                        ),
                        priority_class_name=env.priority_class or None,
                        # OBS joins the ephemeral rpi5 worker ONLY when it's a
                        # software encoder (no iGPU claim): the Pi 5's VideoCore
                        # VII has no H.264 hw encoder, so a VAAPI OBS must stay on
                        # the MS-01's Iris Xe. Gated on `not obs_uses_gpu` so when
                        # obs_gpu flips back to True the affinity drops here AND
                        # the i915 resource claim hard-gates the pod back to the
                        # MS-01. Stage-only via env.prefer_rpi5; see scheduling.py.
                        affinity=prefer_rpi5_affinity()
                        if (env.prefer_rpi5 and not obs_uses_gpu)
                        else None,
                        tolerations=prefer_rpi5_tolerations()
                        if (env.prefer_rpi5 and not obs_uses_gpu)
                        else None,
                        containers=[container],
                    ),
                ),
            ),
        )

        # --- Service ---
        k8s.KubeService(
            self,
            "service",
            metadata=k8s.ObjectMeta(name=name, namespace=ns, labels=labels),
            spec=k8s.ServiceSpec(
                type="ClusterIP",
                selector={"app": name},
                ports=[
                    k8s.ServicePort(
                        name=n,
                        port=c.port(p),
                        target_port=k8s.IntOrString.from_string(n),
                    )
                    for n, p in _PORTS
                ],
            ),
        )

        # --- host-access LoadBalancer (k3d-only convenience: local + dev). The
        # legacy obs-host VNC Service, now per-platform (obs-twitch-host). ---
        if env.cluster in ("local", "k3d"):
            k8s.KubeService(
                self,
                "host-access",
                metadata=k8s.ObjectMeta(name=f"{name}-host", namespace=ns),
                spec=k8s.ServiceSpec(
                    type="LoadBalancer",
                    selector={"app": name},
                    ports=[
                        k8s.ServicePort(
                            name="vnc",
                            port=5902,
                            target_port=k8s.IntOrString.from_string("vnc"),
                        )
                    ],
                ),
            )

        # --- Ingress (noVNC) — only where the env publishes DNS. Overlay-added,
        # so no metadata labels (matches the kustomize render). ---
        if env.dns_base:
            self._ingress(name, env, ns)
        if env.tailscale and env.dns_base:
            self._tailscale_ingress(name, env, ns)

    # ---- helpers ----
    def _external_secret(self, secret_name, sm_path, ns, labels):
        # Stream-key ExternalSecret via the shared typed builder. Overlay-added
        # (prod stream-key), so no metadata labels (matches the render).
        from adanalife_k8s.eso import ESData, external_secret

        external_secret(
            self,
            "stream-key",
            name=secret_name,
            namespace=ns,
            creation_policy="Owner",
            data=[ESData("STREAM_KEY", sm_path)],
        )

    def _ingress(self, name, env: EnvConfig, ns):
        host = f"{name}.{env.dns_base}"
        ann = {"external-dns.alpha.kubernetes.io/hostname": host}
        # minipc envs (prod/stage) get real TLS via the namespaced Route53 issuer;
        # dev is HTTP-only (matches the legacy dev overlay).
        tls = env.cluster == "minipc"
        if tls:
            ann["cert-manager.io/issuer"] = "letsencrypt-route53"
        backend = k8s.IngressBackend(
            service=k8s.IngressServiceBackend(
                name=name, port=k8s.ServiceBackendPort(name="novnc")
            )
        )
        k8s.KubeIngress(
            self,
            "ingress",
            metadata=k8s.ObjectMeta(name=name, namespace=ns, annotations=ann),
            spec=k8s.IngressSpec(
                ingress_class_name="traefik",
                tls=[k8s.IngressTls(hosts=[host], secret_name=f"{name}-tls")]
                if tls
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
                        name=name,
                        port=k8s.ServiceBackendPort(
                            number=load_contract().port("obs_novnc")
                        ),
                    )
                ),
                tls=[k8s.IngressTls(hosts=[f"{name}-{short}"])],
            ),
        )
