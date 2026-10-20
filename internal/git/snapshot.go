package git

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

const (
	binaryCheckBytes = 8000
	maxDiffBytes     = 2 << 20
)

type TreeFile struct {
	Path      string
	ObjectSHA string
	SizeBytes int64
}

// ListTree returns the files at HEAD; an empty repository gives no files
func ListTree(ctx context.Context, runner Runner, dir string) ([]TreeFile, error) {
	headSHA, err := HeadSHA(ctx, runner, dir)
	if err != nil {
		return nil, err
	}
	if headSHA == "" {
		return nil, nil
	}

	output, err := runner.Run(ctx, dir, "ls-tree", "-r", "-l", "-z", "HEAD")
	if err != nil {
		return nil, err
	}

	var files []TreeFile
	for _, entry := range strings.Split(string(output), "\x00") {
		if entry == "" {
			continue
		}
		// Entry format: "<mode> <type> <object> <padded size>\t<path>"
		info, path, found := strings.Cut(entry, "\t")
		fields := strings.Fields(info)
		if !found || len(fields) != 4 {
			return nil, fmt.Errorf("parse ls-tree entry %q", entry)
		}

		isSubmodule := fields[1] != "blob"
		if isSubmodule {
			continue
		}
		sizeBytes, err := strconv.ParseInt(fields[3], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse size of %s: %w", path, err)
		}
		files = append(files, TreeFile{Path: path, ObjectSHA: fields[2], SizeBytes: sizeBytes})
	}
	return files, nil
}

type BlobReader struct {
	cmd    *exec.Cmd
	input  io.WriteCloser
	output *bufio.Reader
}

func OpenBlobReader(ctx context.Context, dir string) (*BlobReader, error) {
	cmd := exec.CommandContext(ctx, "git", "cat-file", "--batch")
	cmd.Dir = dir

	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open cat-file input: %w", err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open cat-file output: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start cat-file: %w", err)
	}
	return &BlobReader{cmd: cmd, input: input, output: bufio.NewReader(output)}, nil
}

func (r *BlobReader) Read(objectSHA string) ([]byte, error) {
	if _, err := io.WriteString(r.input, objectSHA+"\n"); err != nil {
		return nil, fmt.Errorf("request blob %s: %w", objectSHA, err)
	}

	// Header format: "<object> <type> <size>\n" or "<object> missing\n"
	header, err := r.output.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read header of blob %s: %w", objectSHA, err)
	}
	fields := strings.Fields(header)
	if len(fields) != 3 {
		return nil, fmt.Errorf("read blob %s: %s", objectSHA, strings.TrimSpace(header))
	}
	sizeBytes, err := strconv.Atoi(fields[2])
	if err != nil {
		return nil, fmt.Errorf("parse size of blob %s: %w", objectSHA, err)
	}

	contentWithNewline := make([]byte, sizeBytes+1)
	if _, err := io.ReadFull(r.output, contentWithNewline); err != nil {
		return nil, fmt.Errorf("read content of blob %s: %w", objectSHA, err)
	}
	return contentWithNewline[:sizeBytes], nil
}

func (r *BlobReader) Close() error {
	if err := r.input.Close(); err != nil {
		return err
	}
	return r.cmd.Wait()
}

func IsBinaryContent(content []byte) bool {
	checkedPart := content[:min(len(content), binaryCheckBytes)]
	return bytes.IndexByte(checkedPart, 0) >= 0
}

func Show(ctx context.Context, runner Runner, dir, sha string) (diff string, truncated bool, err error) {
	output, err := runner.Run(ctx, dir, "show", "--format=", "--patch", "-M", "--first-parent", sha, "--")
	if err != nil {
		return "", false, err
	}
	if len(output) <= maxDiffBytes {
		return string(output), false, nil
	}

	// Cut after the last full line so the diff viewer does not get half a line
	lastLineEnd := bytes.LastIndexByte(output[:maxDiffBytes], '\n') + 1
	return string(output[:lastLineEnd]), true, nil
}
