"""Scheduling helpers for the ephemeral arm64 rpi5 worker.

`adanalife-rpi5` is a Raspberry Pi 5 joined to the `adanalife-minipc` cluster as
an arm64 worker (infra `talos/adanalife-minipc/worker.patch.yaml`). It's treated
as *ephemeral*: nothing hard-requires it, but chosen stateless stage workloads
PREFER it when it's present and recover onto the MS-01 when it's unplugged.

The node carries:
  - taint  `dana.lol/rpi5=true:NoSchedule`  (repels everything by default,
    so prod pods never drift onto a weak SD-card node)
  - label  `dana.lol/board=rpi5`

A workload opts in (gated by `EnvConfig.prefer_rpi5`, stage-1 only) with two
pieces:

  1. a TOLERATION of the taint, so it's *allowed* to land on the Pi at all; and
  2. a PREFERRED (`preferredDuringScheduling`) node affinity toward the board
     label, so the scheduler *biases* to the Pi but still places the pod on the
     MS-01 when the Pi is absent.

The affinity is deliberately PREFERRED, never REQUIRED: a required term would
pin the pod to the Pi and leave it Pending when the Pi is gone — the opposite of
the ephemeral/recover-to-MS-01 contract. When the Pi is unplugged, the running
pod is evicted by the standard `node.kubernetes.io/unreachable` toleration
(~5 min default) and reschedules onto the MS-01.

OBS opts OUT even in stage: the Pi 5's VideoCore VII has no H.264 hardware
encoder, so OBS must stay on the MS-01's Iris Xe. Its construct simply never
calls these helpers.
"""

from __future__ import annotations

import imports.k8s as k8s

RPI5_TAINT_KEY = "dana.lol/rpi5"
RPI5_BOARD_LABEL = "dana.lol/board"
RPI5_BOARD_VALUE = "rpi5"


def prefer_rpi5_tolerations() -> list[k8s.Toleration]:
    """Toleration for the rpi5 node taint — lets the pod schedule on the Pi."""
    return [
        k8s.Toleration(
            key=RPI5_TAINT_KEY,
            operator="Exists",
            effect="NoSchedule",
        )
    ]


def prefer_rpi5_affinity() -> k8s.Affinity:
    """PREFERRED node affinity toward the rpi5 board label — biases scheduling to
    the Pi when present, falls back to the MS-01 when it's not."""
    return k8s.Affinity(
        node_affinity=k8s.NodeAffinity(
            preferred_during_scheduling_ignored_during_execution=[
                k8s.PreferredSchedulingTerm(
                    weight=100,
                    preference=k8s.NodeSelectorTerm(
                        match_expressions=[
                            k8s.NodeSelectorRequirement(
                                key=RPI5_BOARD_LABEL,
                                operator="In",
                                values=[RPI5_BOARD_VALUE],
                            )
                        ]
                    ),
                )
            ]
        )
    )
