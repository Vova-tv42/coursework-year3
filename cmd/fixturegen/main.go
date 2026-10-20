package main

import (
	"flag"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"coursework/internal/fixture"
)

func main() {
	outDir := flag.String("out", "", "directory to write fixture repositories into")
	flag.Parse()

	if err := run(*outDir); err != nil {
		slog.Error("fixture generation failed", "error", err)
		os.Exit(1)
	}
}

func run(outDir string) error {
	if outDir == "" {
		return fmt.Errorf("-out is required")
	}

	for _, scenario := range slices.Sorted(maps.Keys(fixture.Scenarios)) {
		scenarioDir := filepath.Join(outDir, scenario)
		// A second run would add commits on top of the old ones, so the folder is rebuilt
		if err := os.RemoveAll(scenarioDir); err != nil {
			return fmt.Errorf("clean %s: %w", scenarioDir, err)
		}
		if err := fixture.Build(scenario, scenarioDir); err != nil {
			return err
		}
		fmt.Println(scenarioDir)
	}
	return nil
}
