package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"coursework/internal/git"
	"coursework/internal/metrics"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: analyzer <repo-path>")
		os.Exit(2)
	}

	if err := run(context.Background(), os.Args[1]); err != nil {
		slog.Error("analysis failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, repoPath string) error {
	report, err := metrics.AnalyzeRepository(ctx, git.ExecRunner{}, repoPath)
	if err != nil {
		return err
	}

	output, err := metrics.MarshalReport(report)
	if err != nil {
		return err
	}

	_, err = os.Stdout.Write(output)
	return err
}
