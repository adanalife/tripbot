release-please runs on the automation app token, so the release tag fires `release.yml` by itself (no explicit dispatch) and the Discord announcement only fires on a tag push, never on a re-deploy.
