"""The PreSync image gate — an Argo hook that refuses a sync to an image tag the
registry doesn't have yet.

The release flow bumps `cdk8s/versions.yaml` and pushes the images from the same
release merge, so Argo can reach the new pin before the build has published it.
Cutting 4.14.1 landed that way: `tripbot-tiktok` sat in `Init:ImagePullBackOff`
until the push caught up. The same emitter guards obs and playout; this is
tripbot's copy of it.
"""

from __future__ import annotations

from constructs import Construct

import imports.k8s as k8s

# Multi-arch image carrying the `crane` CLI, used by the gate to probe the
# registry. gcr.io (not Docker Hub) — the CI base-image-mirror policy covers
# builds, not a runtime cluster pull. The version matches the pin obs and
# playout gate on, so one crane serves the whole fleet.
CRANE_IMAGE = "gcr.io/go-containerregistry/crane:v0.21.7"


def emit_image_gate(
    scope: Construct,
    *,
    name: str,
    namespace: str | None,
    labels: dict[str, str],
    image_ref: str,
) -> None:
    """Argo PreSync hook asserting `image_ref` exists in the registry before the
    sync reaches the Deployment.

    A sync to a not-yet-built tag leaves the incoming pod in ImagePullBackOff.
    RollingUpdate (maxUnavailable 0) keeps the outgoing pod serving through
    that, so the stakes are a stuck rollout rather than an outage — tripbot's
    `migrate` initContainer just makes the stuck window a long one. PreSync
    hooks must succeed before the main sync wave, so a `crane manifest` that
    404s fails the hook and aborts the sync, where the miss is visible, instead
    of leaving a half-rolled Deployment to find later. Re-sync once the image
    build lands. Emitted only for pinned (immutable-tag) envs — a floating tag
    always resolves to a prior build, so it can't hit this.
    """
    k8s.KubeJob(
        scope,
        "image-gate",
        metadata=k8s.ObjectMeta(
            name=f"{name}-image-gate",
            namespace=namespace,
            labels=labels,
            annotations={
                "argocd.argoproj.io/hook": "PreSync",
                # Keep the last gate visible for debugging; replaced on next sync.
                "argocd.argoproj.io/hook-delete-policy": "BeforeHookCreation",
            },
        ),
        spec=k8s.JobSpec(
            backoff_limit=2,
            # Cap the wait so a wedged/unschedulable probe fails the sync (pod
            # safe) instead of stalling PreSync forever.
            active_deadline_seconds=120,
            # BeforeHookCreation only clears the previous gate at the next sync,
            # so a finished (especially Failed) gate pod otherwise lingers and
            # clutters every pod-health read. A day leaves the log readable the
            # morning after and reaps it once it isn't.
            ttl_seconds_after_finished=86400,
            template=k8s.PodTemplateSpec(
                # Metadata labels only, never the `app: <name>` selector label:
                # the component's Service selects on that, and a gate pod
                # carrying it would join the Service's endpoints.
                metadata=k8s.ObjectMeta(labels=labels),
                spec=k8s.PodSpec(
                    restart_policy="Never",
                    node_selector={"kubernetes.io/arch": "amd64"},
                    # The `restricted` PodSecurity profile these namespaces run
                    # requires runAsNonRoot as a spec field, whatever USER the
                    # image declares — without it the gate is a violation, which
                    # would fail PreSync for a reason unrelated to the image.
                    # 65532 is crane's own default uid; it only reads the
                    # registry, so the uid is free to state explicitly.
                    security_context=k8s.PodSecurityContext(
                        run_as_non_root=True,
                        run_as_user=65532,
                        seccomp_profile=k8s.SeccompProfile(type="RuntimeDefault"),
                    ),
                    containers=[
                        k8s.Container(
                            name="image-gate",
                            image=CRANE_IMAGE,
                            args=["manifest", image_ref],
                            security_context=k8s.SecurityContext(
                                allow_privilege_escalation=False,
                                capabilities=k8s.Capabilities(drop=["ALL"]),
                            ),
                            resources=k8s.ResourceRequirements(
                                requests={
                                    "cpu": k8s.Quantity.from_string("10m"),
                                    "memory": k8s.Quantity.from_string("32Mi"),
                                },
                                limits={"memory": k8s.Quantity.from_string("64Mi")},
                            ),
                        )
                    ],
                ),
            ),
        ),
    )
