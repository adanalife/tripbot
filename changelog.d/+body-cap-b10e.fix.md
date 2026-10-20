tripbot's HTTP server rejects request bodies over 1 MiB with a 400 instead of reading whatever a caller sends; the largest valid rotator config still fits.
