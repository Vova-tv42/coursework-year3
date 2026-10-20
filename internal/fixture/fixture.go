package fixture

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Author struct {
	Name  string
	Email string
}

var (
	Student        = Author{Name: "Ivan Petrenko", Email: "ivan.petrenko@example.com"}
	QuotedAuthor   = Author{Name: `Mary "Ann" O'Neil`, Email: "mary.oneil@example.com"}
	CyrillicAuthor = Author{Name: "Олександра Коваленко-Шевчук", Email: "o.kovalenko@example.com"}
)

var startDate = time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)

var Scenarios = map[string]func(dir string) error{
	"sizes":                          buildSizes,
	"rapid":                          buildRapid,
	"junk":                           buildJunk,
	"comments":                       buildComments,
	"checklist_complete":             checklistScenario(checklistFiles{code: true, note: true, presentation: true, readme: deployReadme}),
	"checklist_missing_code":         checklistScenario(checklistFiles{note: true, presentation: true, readme: deployReadme}),
	"checklist_missing_note":         checklistScenario(checklistFiles{code: true, presentation: true, readme: deployReadme}),
	"checklist_missing_presentation": checklistScenario(checklistFiles{code: true, note: true, readme: deployReadme}),
	"checklist_missing_readme":       checklistScenario(checklistFiles{code: true, note: true, presentation: true}),
	"checklist_readme_no_deploy":     checklistScenario(checklistFiles{code: true, note: true, presentation: true, readme: plainReadme}),
	"names":                          buildNames,
	"merge_rename":                   buildMergeRename,
	"kyiv_boundary":                  buildKyivBoundary,
	"empty":                          buildEmpty,
}

func Build(scenario, dir string) error {
	build, found := Scenarios[scenario]
	if !found {
		return fmt.Errorf("unknown fixture scenario %q", scenario)
	}
	return build(dir)
}

// repo collects the first error, so scenarios can call its methods one after another and check once
type repo struct {
	dir string
	err error
}

func initRepo(dir string) *repo {
	r := &repo{dir: dir}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		r.err = err
		return r
	}
	r.git(nil, "init", "--quiet", "--initial-branch=main")
	return r
}

func (r *repo) git(env []string, args ...string) {
	if r.err != nil {
		return
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
	// Isolating from the user's git config keeps commit hashes the same on every machine
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	cmd.Env = append(cmd.Env, env...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		r.err = fmt.Errorf("git %s in %s: %w: %s", args[0], r.dir, err, strings.TrimSpace(stderr.String()))
	}
}

func (r *repo) writeFile(path, content string) {
	r.writeBytes(path, []byte(content))
}

func (r *repo) writeBytes(path string, content []byte) {
	if r.err != nil {
		return
	}
	fullPath := filepath.Join(r.dir, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		r.err = err
		return
	}
	r.err = os.WriteFile(fullPath, content, 0o644)
}

func (r *repo) commit(author Author, authoredAt time.Time, message string) {
	r.git(nil, "add", "--all")
	r.git(commitEnv(author, authoredAt), "commit", "--quiet", "--message", message)
}

func commitEnv(author Author, authoredAt time.Time) []string {
	date := authoredAt.Format(time.RFC3339)
	return []string{
		"GIT_AUTHOR_NAME=" + author.Name,
		"GIT_AUTHOR_EMAIL=" + author.Email,
		"GIT_AUTHOR_DATE=" + date,
		"GIT_COMMITTER_NAME=" + author.Name,
		"GIT_COMMITTER_EMAIL=" + author.Email,
		"GIT_COMMITTER_DATE=" + date,
	}
}

func numberedLines(prefix string, count int) string {
	var lines strings.Builder
	for i := 1; i <= count; i++ {
		fmt.Fprintf(&lines, "%s line %d\n", prefix, i)
	}
	return lines.String()
}

// binaryContent starts with a real file signature and contains NUL bytes, so git treats it as binary
func binaryContent(signature string) []byte {
	content := []byte(signature)
	for i := range 64 {
		content = append(content, 0, byte(i))
	}
	return content
}

func buildSizes(dir string) error {
	r := initRepo(dir)
	commitDate := startDate
	for i := 1; i <= 20; i++ {
		r.writeFile(fmt.Sprintf("src/small_%02d.js", i), numberedLines("small", 3))
		r.commit(Student, commitDate, fmt.Sprintf("Add small module %d", i))
		commitDate = commitDate.Add(3 * time.Hour)
	}
	for i := 1; i <= 20; i++ {
		r.writeFile(fmt.Sprintf("src/feature_%02d.js", i), numberedLines("feature", 200))
		r.commit(Student, commitDate, fmt.Sprintf("Add feature %d", i))
		commitDate = commitDate.Add(3 * time.Hour)
	}
	r.writeFile("src/generated_data.js", numberedLines("data", 1500))
	r.commit(Student, commitDate, "Add generated data")
	return r.err
}

func buildRapid(dir string) error {
	r := initRepo(dir)

	// Four commits with gaps of 20 s, 0 s and exactly 60 s form one series
	seriesStart := startDate
	seriesOffsets := []time.Duration{0, 20 * time.Second, 20 * time.Second, 80 * time.Second}
	for i, offset := range seriesOffsets {
		r.writeFile("src/form.js", numberedLines("form", i+1))
		r.commit(Student, seriesStart.Add(offset), fmt.Sprintf("Tweak form %d", i+1))
	}

	fixStart := startDate.Add(24 * time.Hour)
	r.writeFile("src/api.js", numberedLines("api", 5))
	r.commit(Student, fixStart, "Add api client")
	r.writeFile("src/api.js", numberedLines("api", 6))
	r.commit(Student, fixStart.Add(30*time.Second), "Fix api client")

	hourStart := startDate.Add(48 * time.Hour)
	for i := range 4 {
		r.writeFile("src/report.js", numberedLines("report", i+1))
		r.commit(Student, hourStart.Add(time.Duration(i)*20*time.Minute), fmt.Sprintf("Extend report %d", i+1))
	}
	return r.err
}

func buildJunk(dir string) error {
	r := initRepo(dir)
	r.writeFile("src/app.js", numberedLines("app", 10))
	r.writeFile("README.md", "# Junk fixture\n")
	r.commit(Student, startDate, "Add app")

	r.writeFile("node_modules/left-pad/index.js", numberedLines("left-pad", 40))
	r.writeFile("node_modules/left-pad/package.json", "{\n  \"name\": \"left-pad\"\n}\n")
	r.writeFile("vendor/github.com/pkg/errors/errors.go", numberedLines("errors", 30))
	r.commit(Student, startDate.Add(time.Hour), "Commit dependencies")

	r.writeBytes("assets/logo.png", binaryContent("\x89PNG\r\n\x1a\n"))
	r.commit(Student, startDate.Add(2*time.Hour), "Add logo")
	return r.err
}

const phpSource = `<?php
// Greets the user by name
function greet($name) {
    /* Block comment
       on two lines */
    return "Hello, " . $name;
}

echo greet("world");
`

const javaScriptSource = `// Sums two numbers
function sum(a, b) {
  return a + b;
}

/* Exported for tests */
module.exports = { sum };
`

const typeScriptSource = `// Describes a student
interface Student {
  name: string;
  /* Bachelor or master */
  stream: string;
}

export function title(student: Student): string {
  return student.name;
}
`

const pythonSource = `# Prints the total
def total(values):
    """Returns the sum of values."""
    return sum(values)


print(total([1, 2, 3]))
`

const javaSource = `package app;

// Entry point
public class Main {
    /*
     * Prints a greeting.
     */
    public static void main(String[] args) {
        System.out.println("Hello");
    }
}
`

func buildComments(dir string) error {
	r := initRepo(dir)
	r.writeFile("src/index.php", phpSource)
	r.writeFile("src/sum.js", javaScriptSource)
	r.writeFile("src/student.ts", typeScriptSource)
	r.writeFile("src/total.py", pythonSource)
	r.writeFile("src/app/Main.java", javaSource)
	r.commit(Student, startDate, "Add sources in five languages")
	return r.err
}

const deployReadme = `# Thesis project

Web application for the bachelor thesis.

## Deployment

Run docker compose up.
`

const plainReadme = `# Thesis project

Web application for the bachelor thesis.

## Features

Student list and reports.
`

type checklistFiles struct {
	code         bool
	note         bool
	presentation bool
	readme       string
}

func checklistScenario(files checklistFiles) func(dir string) error {
	return func(dir string) error {
		r := initRepo(dir)
		r.writeFile("docs/plan.md", "# Work plan\n")
		if files.code {
			r.writeFile("src/main.go", "package main\n\nfunc main() {\n\tprintln(\"thesis\")\n}\n")
		}
		if files.note {
			r.writeBytes("docs/Пояснювальна записка.pdf", binaryContent("%PDF-1.7\n"))
		}
		if files.presentation {
			r.writeBytes("docs/presentation.pptx", binaryContent("PK\x03\x04"))
		}
		if files.readme != "" {
			r.writeFile("README.md", files.readme)
		}
		r.commit(Student, startDate, "Add thesis files")
		return r.err
	}
}

func buildNames(dir string) error {
	r := initRepo(dir)
	r.writeFile("src/file with spaces.js", numberedLines("spaces", 2))
	r.commit(QuotedAuthor, startDate, `Add "quoted" file with spaces`)

	r.writeFile(`src/say "hello".js`, numberedLines("quotes", 3))
	r.writeFile("документи/звіт.md", "# Звіт\n\nПерший розділ.\n")
	r.commit(CyrillicAuthor, startDate.Add(time.Hour), "Додати звіт\n\nОпис змін у тілі повідомлення.")
	return r.err
}

func buildMergeRename(dir string) error {
	r := initRepo(dir)
	r.writeFile("src/old_name.js", numberedLines("module", 10))
	r.commit(Student, startDate, "Add module")

	r.git(nil, "checkout", "--quiet", "-b", "feature")
	r.git(nil, "mv", "src/old_name.js", "src/new_name.js")
	r.writeFile("src/new_name.js", numberedLines("module", 11))
	r.commit(Student, startDate.Add(time.Hour), "Rename module")

	r.git(nil, "checkout", "--quiet", "main")
	r.writeFile("src/other.js", numberedLines("other", 4))
	r.commit(Student, startDate.Add(2*time.Hour), "Add other module")

	r.git(commitEnv(Student, startDate.Add(3*time.Hour)), "merge", "--quiet", "--no-ff", "--message", "Merge branch feature", "feature")
	return r.err
}

func buildKyivBoundary(dir string) error {
	r := initRepo(dir)
	r.writeFile("src/evening.js", numberedLines("evening", 2))
	r.commit(Student, time.Date(2026, 10, 5, 20, 30, 0, 0, time.UTC), "Evening commit")

	// 23:30 UTC on October 5 is already 02:30 on October 6 in Kyiv; written with the Kyiv offset.
	kyivSummerTime := time.FixedZone("EEST", 3*60*60)
	r.writeFile("src/night.js", numberedLines("night", 3))
	r.commit(Student, time.Date(2026, 10, 6, 2, 30, 0, 0, kyivSummerTime), "Night commit")
	return r.err
}

func buildEmpty(dir string) error {
	return initRepo(dir).err
}
