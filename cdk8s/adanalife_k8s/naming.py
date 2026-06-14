"""Label / selector helpers — one place for the label convention the Kustomize
bases established via `labels: [{includeSelectors: false, pairs: {...}}]`.

That kustomize idiom produces two distinct label sets, which the reference
render makes precise:

  * **metadata labels** (every object) = the `app.kubernetes.io/*` pairs ONLY —
    `includeSelectors: false` keeps the `app:` selector label out of metadata.
  * **selector / pod-template labels** = `app: <name>` ONLY (the base's own
    `spec.selector.matchLabels` + `template.metadata.labels`).

Matching these exactly matters: the Service selector and the Deployment
`matchLabels` are immutable join keys, so a re-apply that changed them would
orphan the running pods. Constructs pass `meta_labels(...)` to every
`metadata.labels` and `selector(...)` to selectors + pod templates.
"""

from __future__ import annotations

from adanalife_k8s.contract import load_contract


def app_name(app: str, platform: str) -> str:
    """The per-platform Kubernetes Service/Deployment name for one app, resolved
    through the contract — e.g. app_name("vlc", "twitch") -> "vlc-twitch".

    `app` is one of "tripbot" / "vlc" / "onscreens" / "obs"; the contract holds
    the canonical `<app>_<platform>` -> `<app>-<platform>` mapping (tripbot's
    pkg/contract is the source of truth, synced via `task contract:sync`). This is
    the single naming entrypoint the app factory routes every workload name
    through, so a rename is one contract edit rather than a sweep across
    constructs.
    """
    return load_contract().svc(f"{app}_{platform}")


def meta_labels(name: str, *, part_of: str = "tripbot") -> dict[str, str]:
    """The `app.kubernetes.io/*` metadata pair kustomize stamped on all objects."""
    return {
        "app.kubernetes.io/name": name,
        "app.kubernetes.io/part-of": part_of,
    }


def selector(name: str) -> dict[str, str]:
    """The `app` label a Service/Deployment selects on, and that pods carry."""
    return {"app": name}
