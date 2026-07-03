// uvb76-vet is a driver for custom analyzers in the UVB-76 codebase.
package main

import (
	"golang.org/x/tools/go/analysis/unitchecker"

	"github.com/s1onique/KGB/uvb76/internal/analyzers/latencyringownership"
)

func main() {
	unitchecker.Main(
		latencyringownership.Analyzer,
	)
}
