package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	recordSeparator    = "\x1e"
	fieldSeparator     = "\x1f"
	logFormat          = "--format=%x1e%H%x1f%P%x1f%an%x1f%ae%x1f%aI%x1f%cI%x1f%B"
	commitHeaderFields = 7
)

type Commit struct {
	SHA          string
	ParentSHAs   []string
	AuthorName   string
	AuthorEmail  string
	AuthoredAt   time.Time
	CommittedAt  time.Time
	Message      string
	IsMerge      bool
	LinesAdded   int
	LinesDeleted int
	Files        []FileChange
}

type FileChange struct {
	Path         string
	OldPath      string
	LinesAdded   int
	LinesDeleted int
	IsBinary     bool
}

// Log returns commits of revRange newest first; an empty repository gives no commits
func Log(ctx context.Context, runner Runner, dir, revRange string) ([]Commit, error) {
	headSHA, err := HeadSHA(ctx, runner, dir)
	if err != nil {
		return nil, err
	}
	if headSHA == "" {
		return nil, nil
	}

	output, err := runner.Run(ctx, dir,
		"-c", "core.quotepath=false",
		"log", "-z", "-M", "--numstat", "--date-order", logFormat, revRange, "--")
	if err != nil {
		return nil, err
	}
	return ParseLog(output)
}

// CommitOrder returns the SHAs reachable from HEAD in the same order as Log
func CommitOrder(ctx context.Context, runner Runner, dir string) ([]string, error) {
	output, err := runner.Run(ctx, dir, "rev-list", "--date-order", "HEAD", "--")
	if err != nil {
		return nil, err
	}

	return strings.Fields(string(output)), nil
}

func ParseLog(output []byte) ([]Commit, error) {
	var commits []Commit
	for _, record := range strings.Split(string(output), recordSeparator) {
		if record == "" {
			continue
		}
		commit, err := parseCommitRecord(record)
		if err != nil {
			return nil, err
		}
		commits = append(commits, commit)
	}
	return commits, nil
}

func parseCommitRecord(record string) (Commit, error) {
	header, numstat, _ := strings.Cut(record, "\x00")
	fields := strings.SplitN(header, fieldSeparator, commitHeaderFields)
	if len(fields) != commitHeaderFields {
		return Commit{}, fmt.Errorf("parse commit header: expected %d fields, got %d", commitHeaderFields, len(fields))
	}

	sha := fields[0]
	authoredAt, err := time.Parse(time.RFC3339, fields[4])
	if err != nil {
		return Commit{}, fmt.Errorf("parse author date of %s: %w", sha, err)
	}
	committedAt, err := time.Parse(time.RFC3339, fields[5])
	if err != nil {
		return Commit{}, fmt.Errorf("parse committer date of %s: %w", sha, err)
	}

	files, err := parseNumstat(numstat)
	if err != nil {
		return Commit{}, fmt.Errorf("parse numstat of %s: %w", sha, err)
	}

	parentSHAs := strings.Fields(fields[1])
	commit := Commit{
		SHA:         sha,
		ParentSHAs:  parentSHAs,
		AuthorName:  fields[2],
		AuthorEmail: fields[3],
		AuthoredAt:  authoredAt,
		CommittedAt: committedAt,
		Message:     strings.TrimRight(fields[6], "\n"),
		IsMerge:     len(parentSHAs) > 1,
		Files:       files,
	}
	for _, file := range files {
		commit.LinesAdded += file.LinesAdded
		commit.LinesDeleted += file.LinesDeleted
	}
	return commit, nil
}

// parseNumstat reads "added\tdeleted\tpath\0" entries; a rename has an empty path followed by "old\0new\0"
func parseNumstat(numstat string) ([]FileChange, error) {
	tokens := strings.Split(strings.TrimPrefix(numstat, "\n"), "\x00")
	var files []FileChange
	for i := 0; i < len(tokens); i++ {
		if tokens[i] == "" {
			continue
		}
		parts := strings.SplitN(tokens[i], "\t", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("unexpected entry %q", tokens[i])
		}

		file := FileChange{Path: parts[2]}
		isRename := file.Path == ""
		if isRename {
			hasRenamePaths := i+2 < len(tokens)
			if !hasRenamePaths {
				return nil, fmt.Errorf("rename entry %q without paths", tokens[i])
			}
			file.OldPath = tokens[i+1]
			file.Path = tokens[i+2]
			i += 2
		}

		file.IsBinary = parts[0] == "-" && parts[1] == "-"
		if !file.IsBinary {
			var err error
			if file.LinesAdded, err = strconv.Atoi(parts[0]); err != nil {
				return nil, fmt.Errorf("parse added lines of %s: %w", file.Path, err)
			}
			if file.LinesDeleted, err = strconv.Atoi(parts[1]); err != nil {
				return nil, fmt.Errorf("parse deleted lines of %s: %w", file.Path, err)
			}
		}
		files = append(files, file)
	}
	return files, nil
}
