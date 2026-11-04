package metrics

import (
	"context"
	"path"
	"slices"
	"strings"

	"coursework/internal/git"
)

const (
	CategoryCode   = "code"
	CategoryDocs   = "docs"
	CategoryTests  = "tests"
	CategoryConfig = "config"
	CategoryBinary = "binary"
)

var (
	testFileMarkers   = []string{"_test.", ".test.", ".spec."}
	testDirectories   = []string{"test", "tests", "__tests__"}
	configExtensions  = []string{".json", ".yml", ".yaml", ".toml", ".xml", ".ini"}
	configFileMarkers = []string{".config."}
)

type SnapshotFile struct {
	Path         string `json:"path"`
	SizeBytes    int64  `json:"sizeBytes"`
	IsBinary     bool   `json:"isBinary"`
	Category     string `json:"category"`
	CodeLines    int    `json:"codeLines"`
	CommentLines int    `json:"commentLines"`
}

// Snapshot holds the files at HEAD and the content of root README files, which the checklist reads
type Snapshot struct {
	Files   []SnapshotFile
	Readmes map[string][]byte
}

// ReadSnapshot reads every file at HEAD once; an empty repository gives an empty snapshot
func ReadSnapshot(ctx context.Context, runner git.Runner, dir string) (Snapshot, error) {
	snapshot := Snapshot{Files: []SnapshotFile{}, Readmes: map[string][]byte{}}
	treeFiles, err := git.ListTree(ctx, runner, dir)
	if err != nil || len(treeFiles) == 0 {
		return snapshot, err
	}

	blobs, err := git.OpenBlobReader(ctx, dir)
	if err != nil {
		return snapshot, err
	}
	defer func() { _ = blobs.Close() }()

	for _, treeFile := range treeFiles {
		content, err := blobs.Read(treeFile.ObjectSHA)
		if err != nil {
			return snapshot, err
		}

		isBinary := git.IsBinaryContent(content)
		snapshot.Files = append(snapshot.Files, SnapshotFile{
			Path:      treeFile.Path,
			SizeBytes: treeFile.SizeBytes,
			IsBinary:  isBinary,
			Category:  Categorize(treeFile.Path, isBinary),
		})

		if isRootReadme(treeFile.Path) && !isBinary {
			snapshot.Readmes[treeFile.Path] = content
		}
	}

	return snapshot, nil
}

// Categorize applies the category rules in order, the first match wins
func Categorize(filePath string, isBinary bool) string {
	switch {
	case isBinary:
		return CategoryBinary
	case strings.EqualFold(path.Ext(filePath), ".md"):
		return CategoryDocs
	case isTestFile(filePath):
		return CategoryTests
	case isConfigFile(filePath):
		return CategoryConfig
	default:
		return CategoryCode
	}
}

func isTestFile(filePath string) bool {
	fileName := path.Base(filePath)
	hasTestMarker := slices.ContainsFunc(testFileMarkers, func(marker string) bool {
		return strings.Contains(fileName, marker)
	})

	parentDirectories := strings.Split(path.Dir(filePath), "/")
	isInTestDirectory := slices.ContainsFunc(parentDirectories, func(directory string) bool {
		return slices.Contains(testDirectories, directory)
	})

	return hasTestMarker || isInTestDirectory
}

func isConfigFile(filePath string) bool {
	fileName := path.Base(filePath)
	hasConfigExtension := slices.Contains(configExtensions, strings.ToLower(path.Ext(fileName)))
	isEnvFile := strings.HasPrefix(fileName, ".env")
	isDockerfile := fileName == "Dockerfile"
	hasConfigMarker := slices.ContainsFunc(configFileMarkers, func(marker string) bool {
		return strings.Contains(fileName, marker)
	})

	return hasConfigExtension || isEnvFile || isDockerfile || hasConfigMarker
}

func isRootReadme(filePath string) bool {
	isInRoot := !strings.Contains(filePath, "/")
	return isInRoot && strings.HasPrefix(strings.ToLower(filePath), "readme")
}
