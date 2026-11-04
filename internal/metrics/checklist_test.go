package metrics_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"coursework/internal/metrics"
)

const (
	codeFile         = "src/main.go"
	noteFile         = "docs/Пояснювальна записка.pdf"
	presentationFile = "docs/presentation.pptx"
	readmeFile       = "README.md"
)

func checklistItems(code, note, presentation, readme []string) []metrics.ChecklistItem {
	newItem := func(key string, files []string) metrics.ChecklistItem {
		if files == nil {
			files = []string{}
		}

		return metrics.ChecklistItem{Key: key, Found: len(files) > 0, Files: files}
	}

	return []metrics.ChecklistItem{
		newItem(metrics.ChecklistCode, code),
		newItem(metrics.ChecklistExplanatoryNote, note),
		newItem(metrics.ChecklistPresentation, presentation),
		newItem(metrics.ChecklistReadmeDeployment, readme),
	}
}

func TestChecklistFixtures(t *testing.T) {
	code, note, presentation, readme := []string{codeFile}, []string{noteFile}, []string{presentationFile}, []string{readmeFile}
	testCases := map[string]metrics.Checklist{
		"checklist_complete":             {Admitted: true, Items: checklistItems(code, note, presentation, readme)},
		"checklist_missing_code":         {Items: checklistItems(nil, note, presentation, readme)},
		"checklist_missing_note":         {Items: checklistItems(code, nil, presentation, readme)},
		"checklist_missing_presentation": {Items: checklistItems(code, note, nil, readme)},
		"checklist_missing_readme":       {Items: checklistItems(code, note, presentation, nil)},
		"checklist_readme_no_deploy":     {Items: checklistItems(code, note, presentation, nil)},
		"empty":                          {Items: checklistItems(nil, nil, nil, nil)},
		"junk":                           {Items: checklistItems([]string{"src/app.js"}, nil, nil, nil)},
	}

	for scenario, expected := range testCases {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, expected, analyzeFixture(t, scenario).Checklist)
		})
	}
}

func TestDeploymentHeadingDetection(t *testing.T) {
	testCases := map[string]struct {
		readme  string
		isFound bool
	}{
		"atx heading":               {readme: "# App\n\n## Installation\n\nRun it.\n", isFound: true},
		"setext heading":            {readme: "App\n===\n\nGetting started\n---------------\n", isFound: true},
		"ukrainian heading":         {readme: "# Застосунок\n\n## Розгортання на сервері\n", isFound: true},
		"comment inside code block": {readme: "# App\n\n```bash\n# install dependencies\nnpm ci\n```\n", isFound: false},
		"word only in a paragraph":  {readme: "# App\n\nTo deploy, ask the author.\n", isFound: false},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			snapshot := metrics.Snapshot{Readmes: map[string][]byte{readmeFile: []byte(testCase.readme)}}
			readmeItem := metrics.EvaluateChecklist(snapshot).Items[3]

			require.Equal(t, metrics.ChecklistReadmeDeployment, readmeItem.Key)
			require.Equal(t, testCase.isFound, readmeItem.Found)
		})
	}
}

func TestChecklistFilesAreLimited(t *testing.T) {
	var files []metrics.SnapshotFile
	for range metrics.MaxChecklistFiles + 5 {
		files = append(files, metrics.SnapshotFile{Path: "src/main.go", Category: metrics.CategoryCode})
	}

	codeItem := metrics.EvaluateChecklist(metrics.Snapshot{Files: files}).Items[0]
	require.True(t, codeItem.Found)
	require.Len(t, codeItem.Files, metrics.MaxChecklistFiles)
}

func TestCategorize(t *testing.T) {
	testCases := []struct {
		path     string
		isBinary bool
		category string
	}{
		{path: "docs/logo.png", isBinary: true, category: metrics.CategoryBinary},
		{path: "README.md", category: metrics.CategoryDocs},
		{path: "tests/README.md", category: metrics.CategoryDocs},
		{path: "internal/api/students_test.go", category: metrics.CategoryTests},
		{path: "src/app.test.ts", category: metrics.CategoryTests},
		{path: "src/app.spec.js", category: metrics.CategoryTests},
		{path: "test/helpers.py", category: metrics.CategoryTests},
		{path: "app/tests/models.py", category: metrics.CategoryTests},
		{path: "src/__tests__/form.jsx", category: metrics.CategoryTests},
		{path: "package.json", category: metrics.CategoryConfig},
		{path: ".github/workflows/ci.yml", category: metrics.CategoryConfig},
		{path: "config/app.yaml", category: metrics.CategoryConfig},
		{path: "pyproject.toml", category: metrics.CategoryConfig},
		{path: "pom.xml", category: metrics.CategoryConfig},
		{path: "php.ini", category: metrics.CategoryConfig},
		{path: ".env.example", category: metrics.CategoryConfig},
		{path: "Dockerfile", category: metrics.CategoryConfig},
		{path: "vite.config.ts", category: metrics.CategoryConfig},
		{path: "src/main.go", category: metrics.CategoryCode},
		{path: "src/testing.go", category: metrics.CategoryCode},
		{path: "Makefile", category: metrics.CategoryCode},
	}

	for _, testCase := range testCases {
		t.Run(testCase.path, func(t *testing.T) {
			require.Equal(t, testCase.category, metrics.Categorize(testCase.path, testCase.isBinary))
		})
	}
}

func TestIsJunkWithDefaultPatterns(t *testing.T) {
	testCases := []struct {
		file   metrics.SnapshotFile
		isJunk bool
	}{
		{file: metrics.SnapshotFile{Path: "node_modules/left-pad/index.js"}, isJunk: true},
		{file: metrics.SnapshotFile{Path: "web/node_modules/react/index.js"}, isJunk: true},
		{file: metrics.SnapshotFile{Path: "vendor/github.com/pkg/errors/errors.go"}, isJunk: true},
		{file: metrics.SnapshotFile{Path: "web/dist/assets/index.js"}, isJunk: true},
		{file: metrics.SnapshotFile{Path: "target/classes/App.class"}, isJunk: true},
		{file: metrics.SnapshotFile{Path: "app/__pycache__/main.cpython-312.pyc"}, isJunk: true},
		{file: metrics.SnapshotFile{Path: "lib/helper.jar"}, isJunk: true},
		{file: metrics.SnapshotFile{Path: "assets/logo.png", IsBinary: true}, isJunk: true},
		{file: metrics.SnapshotFile{Path: "src/app.js"}, isJunk: false},
		{file: metrics.SnapshotFile{Path: "src/vendors.js"}, isJunk: false},
		{file: metrics.SnapshotFile{Path: "src/build.go"}, isJunk: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.file.Path, func(t *testing.T) {
			require.Equal(t, testCase.isJunk, metrics.IsJunk(testCase.file, metrics.DefaultJunkPatterns))
		})
	}
}

func TestIsJunkWithCustomPatterns(t *testing.T) {
	binaryFile := metrics.SnapshotFile{Path: "assets/logo.png", IsBinary: true}
	generatedFile := metrics.SnapshotFile{Path: "src/api/schema.gen.ts"}

	require.False(t, metrics.IsJunk(binaryFile, []string{"**/*.gen.ts"}))
	require.True(t, metrics.IsJunk(generatedFile, []string{"**/*.gen.ts"}))
	require.False(t, metrics.IsJunk(generatedFile, []string{"[unclosed"}))
	require.False(t, metrics.IsJunk(generatedFile, nil))
}
