#!/usr/bin/env python
"""cdk8s entrypoint for the tripbot app workloads. Synthesizes the deploy units
into dist/.

Per env:  <env>-<component>-<platform>  — one deploy unit / Argo Application each
                                           (tripbot/vlc/onscreens/obs × platforms)
          <env>-tripbot-identity         — identity Secrets + prod-stream protection
          <env>-job-<name>               — tripbot one-shot Jobs (auth/seed; their own tasks)

The stateful + shared-platform units (postgres, ESO SecretStore, shared
observability Secrets, cert-manager Issuers, dashcam PV/PVC, the Argo config) stay
in infra/cdk8s. Argo delivers these dist files cross-repo (the tripbot-apps /
tripbot-identity ApplicationSets in infra point at this repo).

Synth all envs by default; CDK8S_ENV=<name> narrows to one (handy for diffing a
single env's output).
"""

import os

from cdk8s import App

from adanalife_k8s.charts import IdentityChart, emit_app_charts, emit_job_charts
from adanalife_k8s.config import ENVS, load_env

# outdir honors CDK8S_OUTDIR so a caller can synth to a throwaway dir without
# touching the committed dist/.
app = App(outdir=os.environ.get("CDK8S_OUTDIR", "dist"))

only = os.environ.get("CDK8S_ENV")
targets = [only] if only else list(ENVS)
for name in targets:
    env = load_env(name)
    # One Chart per (component, platform) -> dist/<env>-<component>-<platform>.k8s.yaml.
    emit_app_charts(app, env)
    # Identity Secrets + prod-stream protection — its own deploy unit / Argo
    # Application, isolated from the per-component app churn.
    IdentityChart(app, f"{name}-tripbot-identity", env=env)
    # One-shot Jobs (auth/seed) — one dist file each, applied on demand by tasks.
    emit_job_charts(app, env)

app.synth()
