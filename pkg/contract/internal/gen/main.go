// Command gen marshals the canonical pkg/contract constants to
// pkg/contract/contract.json. It is invoked by the //go:generate directive in
// pkg/contract/generate.go; run `go generate ./pkg/contract` after editing the
// constants.
package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/adanalife/tripbot/pkg/contract"
)

func main() {
	data, err := contract.Current().Marshal()
	if err != nil {
		log.Fatalf("marshal contract: %v", err)
	}

	// Resolve the output path relative to this source file so the generator
	// writes to pkg/contract/contract.json regardless of the working directory
	// `go generate` runs from.
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatal("could not determine generator source path")
	}
	// self = pkg/contract/internal/gen/main.go → pkg/contract/contract.json
	out := filepath.Join(filepath.Dir(self), "..", "..", "contract.json")

	if err := os.WriteFile(out, data, 0o644); err != nil {
		log.Fatalf("write %s: %v", out, err)
	}
	log.Printf("wrote %s", out)
}
