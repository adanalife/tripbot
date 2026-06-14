"""Prod-stream protection objects (2026-06-11 stage-starves-prod incident).

Extracted from infra/cdk8s's supporting.py: just the PriorityClass + ResourceQuota
half (k8s-typed, no cert-manager), which is tripbot-owned. The shared observability
Secrets + cert-manager Issuers stay in infra's SupportingChart. Emitted into the
per-env tripbot-identity unit (charts.IdentityChart) alongside the identity Secrets.
"""

from __future__ import annotations

from constructs import Construct

import imports.k8s as k8s
from adanalife_k8s.config import EnvConfig


def emit_stream_protection(scope: Construct, env: EnvConfig) -> None:
    """The prod-stream protection objects: the PriorityClass the env's app pods
    reference (when env.priority_class is set), and the app-namespace ResourceQuota
    capping what co-tenant envs can request in aggregate (when env.app_quota is set).
    """
    ns = env.namespace or None

    if env.priority_class:
        # Cluster-scoped; cdk8s stamps the chart namespace, which the apiserver
        # ignores on cluster-scoped kinds (same as dashcam-cv-low). value 1000
        # outranks every default-priority (0) pod, so under node pressure the
        # scheduler preempts co-tenants, never the live stream. dashcam-cv-low
        # (-10) stays the most-preemptible tier.
        k8s.KubePriorityClass(
            scope,
            "priority-class",
            metadata=k8s.ObjectMeta(name=env.priority_class),
            value=1000,
            global_default=False,
            description="Live-stream workloads — outrank default-priority co-tenants; preempt them under node pressure.",
        )

    if env.app_quota:
        k8s.KubeResourceQuota(
            scope,
            "app-quota",
            metadata=k8s.ObjectMeta(name="app-quota", namespace=ns),
            spec=k8s.ResourceQuotaSpec(
                hard={
                    key: k8s.Quantity.from_string(val)
                    for key, val in env.app_quota.items()
                }
            ),
        )
