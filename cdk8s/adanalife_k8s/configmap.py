"""ConfigMap helpers: dict merge (matches Kustomize `behavior: merge`) + a
stable-name emitter that returns a content hash for the pod-template annotation.

The plan's deliberate divergence from Kustomize: Kustomize name-hashes the
ConfigMap (`tripbot-config-tkd4fgtt6c`) and rewrites every reference to it.
cdk8s instead keeps the **stable logical name** (`tripbot-config`) and stamps
a short content hash as `adanalife.dev/config-hash` on the pod template, so the
Deployment still rolls on config change but jobs/`envFrom` references that target
the name (e.g. the stable `tripbot-config`) never break.
"""

from __future__ import annotations

import hashlib
import json

from constructs import Construct

import imports.k8s as k8s

ANNOTATION = "adanalife.dev/config-hash"


def merge(*layers: dict[str, str]) -> dict[str, str]:
    """Right-most-wins shallow merge (Kustomize `behavior: merge` semantics)."""
    out: dict[str, str] = {}
    for layer in layers:
        out.update(layer)
    return out


def content_hash(data: dict[str, str]) -> str:
    """Stable 10-char hash of ConfigMap data, for the pod-template annotation."""
    return hashlib.sha256(json.dumps(data, sort_keys=True).encode()).hexdigest()[:10]


def config_map(
    scope: Construct,
    id: str,
    *,
    name: str,
    namespace: str | None,
    labels: dict[str, str],
    data: dict[str, str],
) -> str:
    """Emit a stable-named ConfigMap; return its content hash so the caller can
    annotate the pod template (`pod_annotations(hash)`)."""
    k8s.KubeConfigMap(
        scope,
        id,
        metadata=k8s.ObjectMeta(name=name, namespace=namespace, labels=labels),
        data=data,
    )
    return content_hash(data)


def pod_annotations(cfg_hash: str) -> dict[str, str]:
    """The pod-template annotation that rolls the Deployment on config change."""
    return {ANNOTATION: cfg_hash}
