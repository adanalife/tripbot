The migrate init container and the seed Job's helper containers carry a 512Mi memory limit, so every tripbot Deployment passes the cluster's require-requests-limits policy.
