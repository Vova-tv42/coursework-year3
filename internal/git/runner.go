package git

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Runner interface {
	Run(ctx context.Context, dir string, args ...string) ([]byte, error)
}

type ExecRunner struct {
	Token string
}

func (r ExecRunner) Run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	commandArgs := args
	if r.Token != "" {
		credentials := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + r.Token))
		authHeader := "http.extraHeader=Authorization: Basic " + credentials
		commandArgs = append([]string{"-c", authHeader}, args...)
	}

	cmd := exec.CommandContext(ctx, "git", commandArgs...)
	cmd.Dir = dir
	// Without this git waits for a password on stdin when a private repository rejects the token
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return output, nil
}

func CloneBare(ctx context.Context, runner Runner, url, dir string) error {
	// "--" keeps a URL that starts with a dash from being read as an option
	_, err := runner.Run(ctx, "", "clone", "--bare", "--quiet", "--", url, dir)
	return err
}

func Fetch(ctx context.Context, runner Runner, dir string) error {
	_, err := runner.Run(ctx, dir, "fetch", "--prune", "--quiet", "origin", "+refs/heads/*:refs/heads/*")
	return err
}

// HeadSHA returns an empty string for a repository without commits
func HeadSHA(ctx context.Context, runner Runner, dir string) (string, error) {
	output, err := runner.Run(ctx, dir, "rev-parse", "--verify", "--quiet", "HEAD")
	if hasExitCode(err, 1) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func DefaultBranch(ctx context.Context, runner Runner, dir string) (string, error) {
	output, err := runner.Run(ctx, dir, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func IsAncestor(ctx context.Context, runner Runner, dir, ancestorSHA, descendantSHA string) (bool, error) {
	_, err := runner.Run(ctx, dir, "merge-base", "--is-ancestor", ancestorSHA, descendantSHA)
	if hasExitCode(err, 1) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func DirSize(dir string) (int64, error) {
	var totalBytes int64
	err := filepath.WalkDir(dir, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		totalBytes += info.Size()
		return nil
	})
	return totalBytes, err
}

func hasExitCode(err error, code int) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == code
}
