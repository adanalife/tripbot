# cdk8s — tripbot app workload manifests (Python)

The Kubernetes manifests for the four images built from this repo — **tripbot,
vlc, onscreens, obs** — authored as typed cdk8s constructs and synthesized to
plain YAML under `dist/`. Argo delivers those committed files cross-repo (the
`tripbot-apps` / `tripbot-identity` ApplicationSets live in `infra`, pointed at
this repo). Keeping the manifests here means an env-var / config change rides in
the same repo (and shows up in the same release) as the code + version bump that
needs it.

**What stays in `infra/cdk8s`:** the stateful + shared-platform layer — postgres
(`DataChart`), the ESO `SecretStore`, the shared observability Secrets +
cert-manager Issuers (`SupportingChart`), the dashcam PV/PVC, and the Argo config
itself. The charts here reference the Secrets those units materialize *by name*
(`grafana-cloud-otlp`, `sentry-*`, the DB creds) — that naming is the contract
between the two repos.

## Setup

Tools are pinned via mise (`.mise.toml`): python, node, cdk8s-cli. Python deps via
uv.

```bash
mise install                 # python, node, cdk8s-cli
uv sync                      # python deps into .venv
mise exec -- uv run cdk8s import   # typed k8s + ESO constructs into imports/ (gitignored)
```

## Develop

```bash
mise exec -- uv run cdk8s synth                  # -> dist/<env>-<component>-<platform>.k8s.yaml (+ -tripbot-identity / -job-*)
CDK8S_ENV=stage-1 mise exec -- uv run cdk8s synth    # one env
mise exec -- uv run pytest -q                    # synth-time checks on dist/
```

Run these via the repo-root Taskfile: `task cdk8s:imports`, `task cdk8s:synth`,
`task cdk8s:test`, `task cdk8s:check` (golden-file gate — re-synths and fails if
`dist/` is stale).

## Versioning

`versions.yaml` holds the per-component **prod-1** image pins. prod deploys those
exact tags (`IfNotPresent`); stage/dev/local float on `EnvConfig.image_tag`
(`main` / `latest`, `Always`). release-please bumps the pins on its standing
release PR (the `x-release-please-version` markers) and that PR carries the
re-synthed `dist/`, so merging the release PR is the prod deploy — see
`.github/workflows/release-please.yml`.

`contract.json` is read directly from this repo's `pkg/contract` (the Go source of
truth) — no sync step.
