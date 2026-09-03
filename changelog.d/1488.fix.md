`task test` now rebuilds its compose test image against the latest mirrored Go base and fails with a clear message when that image's toolchain is older than `go.mod` requires.
