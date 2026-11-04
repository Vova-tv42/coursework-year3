package metrics

import (
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
)

const (
	ChecklistCode             = "code"
	ChecklistExplanatoryNote  = "explanatory_note"
	ChecklistPresentation     = "presentation"
	ChecklistReadmeDeployment = "readme_deployment"
)

// MaxChecklistFiles limits the matched files kept per item, because the code item can match thousands of files
const MaxChecklistFiles = 20

var (
	noteExtensions           = []string{".pdf", ".docx", ".doc", ".odt"}
	noteNameHints            = []string{"записк", "пояснюв", "note", "explanatory", "thesis", "диплом", "report"}
	presentationExtensions   = []string{".pptx", ".ppt", ".odp", ".key"}
	presentationNameHints    = []string{"презент", "presentation", "slides"}
	deploymentHeadingPattern = regexp.MustCompile(
		`(?i)deploy|install|setup|getting started|розгортання|встановлення|інсталяц|запуск`)
)

type ChecklistItem struct {
	Key   string   `json:"key"`
	Found bool     `json:"found"`
	Files []string `json:"files"`
}

type Checklist struct {
	Admitted bool            `json:"admitted"`
	Items    []ChecklistItem `json:"items"`
}

// EvaluateChecklist checks the defense requirements on the HEAD snapshot with the default junk patterns
func EvaluateChecklist(snapshot Snapshot) Checklist {
	items := []ChecklistItem{
		newChecklistItem(ChecklistCode, matchingPaths(snapshot.Files, isProjectCode)),
		newChecklistItem(ChecklistExplanatoryNote, matchingPaths(snapshot.Files, isExplanatoryNote)),
		newChecklistItem(ChecklistPresentation, matchingPaths(snapshot.Files, isPresentation)),
		newChecklistItem(ChecklistReadmeDeployment, readmesWithDeployment(snapshot.Readmes)),
	}

	isEveryItemFound := !slices.ContainsFunc(items, func(item ChecklistItem) bool { return !item.Found })
	return Checklist{Admitted: isEveryItemFound, Items: items}
}

func newChecklistItem(key string, files []string) ChecklistItem {
	return ChecklistItem{Key: key, Found: len(files) > 0, Files: files[:min(len(files), MaxChecklistFiles)]}
}

func matchingPaths(files []SnapshotFile, matches func(SnapshotFile) bool) []string {
	paths := []string{}
	for _, file := range files {
		if matches(file) {
			paths = append(paths, file.Path)
		}
	}

	return paths
}

func isProjectCode(file SnapshotFile) bool {
	isCodeOrTests := file.Category == CategoryCode || file.Category == CategoryTests
	return isCodeOrTests && !IsJunk(file, DefaultJunkPatterns)
}

func isExplanatoryNote(file SnapshotFile) bool {
	return hasExtension(file.Path, noteExtensions) && hasNameHint(file.Path, noteNameHints)
}

func isPresentation(file SnapshotFile) bool {
	isPresentationDocument := hasExtension(file.Path, presentationExtensions)
	isPresentationPDF := hasExtension(file.Path, []string{".pdf"}) && hasNameHint(file.Path, presentationNameHints)
	return isPresentationDocument || isPresentationPDF
}

func hasExtension(filePath string, extensions []string) bool {
	return slices.Contains(extensions, strings.ToLower(path.Ext(filePath)))
}

func hasNameHint(filePath string, hints []string) bool {
	fileName := strings.ToLower(path.Base(filePath))
	return slices.ContainsFunc(hints, func(hint string) bool { return strings.Contains(fileName, hint) })
}

func readmesWithDeployment(readmes map[string][]byte) []string {
	paths := []string{}
	for _, readmePath := range slices.Sorted(maps.Keys(readmes)) {
		if hasDeploymentHeading(readmes[readmePath]) {
			paths = append(paths, readmePath)
		}
	}

	return paths
}

// hasDeploymentHeading parses Markdown, so a "# install" line inside a code block is not taken for a heading
func hasDeploymentHeading(markdown []byte) bool {
	document := parser.New().Parse(markdown)
	isFound := false
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		heading, isHeading := node.(*ast.Heading)
		if !entering || !isHeading {
			return ast.WalkContinue, nil
		}

		for _, segment := range heading.Source() {
			if deploymentHeadingPattern.MatchString(segment.Str(markdown)) {
				isFound = true
				return ast.WalkStop, nil
			}
		}

		return ast.WalkSkipChildren, nil
	})

	return isFound
}
