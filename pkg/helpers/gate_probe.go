package helpers

// gateProbeDeadFunc exists to prove the staticcheck job fails on a finding.
// Removed in the next commit.
func gateProbeDeadFunc() int { return 1 }
