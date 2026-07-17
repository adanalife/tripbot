The cdk8s synth now emits a per-app discovery index at `dist/apps/<env>-<app>.json`, so infra's tripbot-apps ApplicationSet can self-discover deploy units instead of duplicating the platform matrix.
