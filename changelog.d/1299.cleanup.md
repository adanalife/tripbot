`config.Load` for onscreens-server returns an error instead of calling `log.Fatalf` from inside the package, so main owns the exit and the function is unit-testable. Its first two tests come with it.
