"""Co-tenant ResourceQuota (2026-06-11 stage-starves-prod incident).

The app-namespace ResourceQuota that caps what co-tenant envs can request in
aggregate. Emitted into the per-env tripbot-identity unit (charts.IdentityChart)
alongside the identity Secrets.

The scheduling priority tiers (prod-stream / prod-support) that used to live here
are owned by infra now (adanalife_k8s/priority.py, delivered by the prod-1
SupportingChart) — cluster-wide policy referenced by name across every app repo.
This module keeps only the namespace-scoped quota half; app pods still set
priorityClassName=env.priority_class, referencing the infra-owned class.
"""

from __future__ import annotations

from constructs import Construct

import imports.k8s as k8s
from adanalife_k8s.config import EnvConfig


def emit_stream_protection(scope: Construct, env: EnvConfig) -> None:
    """The app-namespace ResourceQuota capping what co-tenant envs can request in
    aggregate (when env.app_quota is set). The priority tiers app pods reference
    are owned by infra (adanalife_k8s/priority.py); see the module docstring.
    """
    ns = env.namespace or None

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
