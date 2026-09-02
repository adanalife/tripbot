Drop the dead `cache_from: adanalife/tripbot` from the CI compose override — the image is pre-built and loaded before compose runs, so the cache key was never consulted.
