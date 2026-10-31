"""Ingress allowlist for the tripbot and onscreens-server HTTP ports.

Both serve unauthenticated /api writes that change what is on the live stream,
and the cluster is shared by stage, prod and the self-hosted CI runners, so
ClusterIP alone is not a boundary. Admitted:

  * the env's own namespace — the console, OBS (its browser sources load the
    onscreens overlay pages) and tripbot → onscreens. Peers are selected by namespace, not pod label: app pods carry only
    `app: <name>`, and a label-based peer on the gateway policy silently
    dropped prod chat (2026-10-14).
  * `monitoring` — Grafana's PDC agent, which backs the `tripbot-api`
    datasource (infra terraform/platform/grafana-tripbot-api.tf), and Alloy.

Traffic from the node itself (kubelet probes, `kubectl port-forward`, traefik
under hostNetwork) carries Cilium's host identity, which is admitted regardless
of policy. Anything else is dropped silently: a new caller needs a rule here,
and the symptom of a missing one is a connect timeout, not an error.
"""

from __future__ import annotations

from constructs import Construct

import imports.k8s as k8s

_ALLOWED_NAMESPACES = ("monitoring",)


def emit_http_ingress_policy(
    scope: Construct,
    *,
    name: str,
    namespace: str,
    labels: dict[str, str],
    selector: dict[str, str],
    port: int,
) -> None:
    peers = [
        k8s.NetworkPolicyPeer(
            namespace_selector=k8s.LabelSelector(
                match_labels={"kubernetes.io/metadata.name": ns}
            )
        )
        for ns in (namespace, *_ALLOWED_NAMESPACES)
    ]
    k8s.KubeNetworkPolicy(
        scope,
        "network-policy",
        metadata=k8s.ObjectMeta(name=name, namespace=namespace, labels=labels),
        spec=k8s.NetworkPolicySpec(
            pod_selector=k8s.LabelSelector(match_labels=selector),
            # Ingress only: egress (NATS, Postgres, OTLP, the platform APIs)
            # stays open.
            policy_types=["Ingress"],
            ingress=[
                k8s.NetworkPolicyIngressRule(
                    from_=peers,
                    ports=[
                        k8s.NetworkPolicyPort(
                            port=k8s.IntOrString.from_number(port), protocol="TCP"
                        )
                    ],
                )
            ],
        ),
    )
