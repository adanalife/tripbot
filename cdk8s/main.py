#!/usr/bin/env python
"""cdk8s entrypoint for the tripbot app workloads. Synthesizes the deploy units
into dist/.

Per env:  <env>-<component>-<platform>  — one deploy unit / Argo Application each
                                           (tripbot/onscreens × platforms)
          <env>-tripbot-identity         — identity Secrets + prod-stream protection
          <env>-job-<name>               — tripbot one-shot Jobs (auth/seed; their own tasks)

The stateful + shared-platform units (postgres, ESO SecretStore, shared
observability Secrets, cert-manager Issuers, dashcam PV/PVC, the Argo config) stay
in infra/cdk8s. Argo delivers these dist files cross-repo (the tripbot-apps /
tripbot-identity ApplicationSets in infra point at this repo).

Synth all envs by default; CDK8S_ENV=<name> narrows to one (handy for diffing a
single env's output).
"""

import json
import os

from cdk8s import App

from adanalife_k8s.charts import (
    IdentityChart,
    app_unit_names,
    emit_app_charts,
    emit_job_charts,
)
from adanalife_k8s.config import ENVS, load_env

# outdir honors CDK8S_OUTDIR so a caller can synth to a throwaway dir without
# touching the committed dist/.
outdir = os.environ.get("CDK8S_OUTDIR", "dist")
app = App(outdir=outdir)

only = os.environ.get("CDK8S_ENV")
targets = [only] if only else list(ENVS)
index_entries: list[dict[str, str]] = []
for name in targets:
    env = load_env(name)
    # One Chart per (component, platform) -> dist/<env>-<component>-<platform>.k8s.yaml.
    emit_app_charts(app, env)
    # Identity Secrets + prod-stream protection — its own deploy unit / Argo
    # Application, isolated from the per-component app churn.
    IdentityChart(app, f"{name}-tripbot-identity", env=env)
    # One-shot Jobs (auth/seed) — one dist file each, applied on demand by tasks.
    emit_job_charts(app, env)
    index_entries += [{"env": name, "app": unit} for unit in app_unit_names(env)]

app.synth()

# Discovery index for infra's tripbot-apps ApplicationSet: one tiny JSON per app
# unit at dist/apps/<env>-<app>.json. The appset's git files generator globs
# these instead of infra re-declaring the (env × component × platform) matrix,
# so the deploy units the appset delivers can never drift from the ones synthed
# here. Written after synth (plain files, not cdk8s objects). Deterministic
# bytes so the golden gate is stable.
apps_dir = os.path.join(outdir, "apps")
os.makedirs(apps_dir, exist_ok=True)
for entry in index_entries:
    path = os.path.join(apps_dir, f"{entry['env']}-{entry['app']}.json")
    with open(path, "w") as f:
        f.write(json.dumps(entry, indent=2, sort_keys=True) + "\n")
