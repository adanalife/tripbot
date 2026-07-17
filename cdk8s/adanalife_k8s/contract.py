"""Loader for contract.json — the canonical service names / ports / env keys.

This lives in the tripbot repo, so the source of truth (`pkg/contract`, generated
by `go generate ./pkg/contract`) is read DIRECTLY — no `task contract:sync` copy
step like infra/cdk8s needs. Constructs reference these instead of hard-coding
strings, so a rename/port change is one edit on the Go side and any mismatch is
caught by `go generate`'s own anti-drift check.
"""

from __future__ import annotations

import json
from functools import cache
from pathlib import Path

# repo-root/pkg/contract/contract.json — contract.py sits at
# repo-root/cdk8s/adanalife_k8s/contract.py, so the repo root is parents[2].
_CONTRACT_PATH = (
    Path(__file__).resolve().parents[2] / "pkg" / "contract" / "contract.json"
)


class Contract:
    def __init__(self, raw: dict):
        self.services: dict[str, str] = raw["services"]
        self.ports: dict[str, int] = raw["ports"]
        self.env_keys: dict[str, str] = raw["env_keys"]

    def svc(self, key: str) -> str:
        return self.services[key]

    def port(self, key: str) -> int:
        return self.ports[key]


@cache
def load_contract() -> Contract:
    return Contract(json.loads(_CONTRACT_PATH.read_text()))
