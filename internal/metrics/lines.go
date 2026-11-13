package metrics

import (
	"bytes"
	"path"
	"strings"

	"github.com/hhatto/gocloc"
)

const MaxJunkSamplePaths = 50

// gocloc.Exts has no "ts" key, so the languages with a comment split are mapped by extension here
var commentLanguageByExtension = func() map[string]*gocloc.Language {
	languages := gocloc.NewDefinedLanguages().Langs
	return map[string]*gocloc.Language{
		".php":  languages["PHP"],
		".js":   languages["JavaScript"],
		".jsx":  languages["JSX"],
		".mjs":  languages["JavaScript"],
		".cjs":  languages["JavaScript"],
		".ts":   languages["TypeScript"],
		".tsx":  languages["TypeScript"],
		".py":   languages["Python"],
		".java": languages["Java"],
	}
}()

var clocOptions = gocloc.NewClocOptions()

type LineCount struct {
	Code     int
	Comments int
}

type LineCounts struct {
	Code     int `json:"code"`
	Comments int `json:"comments"`
	Docs     int `json:"docs"`
	Tests    int `json:"tests"`
	Config   int `json:"config"`
	Total    int `json:"total"`
}

type JunkVolume struct {
	Files       int      `json:"files"`
	Bytes       int64    `json:"bytes"`
	Lines       int      `json:"lines"`
	SamplePaths []string `json:"samplePaths"`
}

type CodeStatistics struct {
	Lines LineCounts `json:"lines"`
	Junk  JunkVolume `json:"junk"`
}

// CountLines skips blank lines; comments are split only for the languages in commentLanguageByExtension
func CountLines(filePath string, content []byte) LineCount {
	language, hasCommentSplit := commentLanguageByExtension[strings.ToLower(path.Ext(filePath))]
	if !hasCommentSplit {
		return LineCount{Code: countNonBlankLines(content)}
	}

	counted := gocloc.AnalyzeReader(filePath, language, bytes.NewReader(content), clocOptions)
	return LineCount{Code: int(counted.Code), Comments: int(counted.Comments)}
}

func countNonBlankLines(content []byte) int {
	nonBlankLines := 0
	for line := range bytes.Lines(content) {
		if len(bytes.TrimSpace(line)) > 0 {
			nonBlankLines++
		}
	}
	return nonBlankLines
}

// CodeStats splits comments only for code files; comments in tests, docs and config count in their category
func CodeStats(files []SnapshotFile, junkPatterns []string) CodeStatistics {
	stats := CodeStatistics{Junk: JunkVolume{SamplePaths: []string{}}}

	for _, file := range files {
		fileLines := file.CodeLines + file.CommentLines
		if IsJunk(file, junkPatterns) {
			stats.Junk.Files++
			stats.Junk.Bytes += file.SizeBytes
			stats.Junk.Lines += fileLines
			if len(stats.Junk.SamplePaths) < MaxJunkSamplePaths {
				stats.Junk.SamplePaths = append(stats.Junk.SamplePaths, file.Path)
			}
			continue
		}

		switch file.Category {
		case CategoryCode:
			stats.Lines.Code += file.CodeLines
			stats.Lines.Comments += file.CommentLines
		case CategoryDocs:
			stats.Lines.Docs += fileLines
		case CategoryTests:
			stats.Lines.Tests += fileLines
		case CategoryConfig:
			stats.Lines.Config += fileLines
		}
		stats.Lines.Total += fileLines
	}

	return stats
}
