onscreens-server no longer serves `POST /admin/shutdown`: nothing called it, and any pod in the cluster could restart the overlays with one unauthenticated request.
