package arch

import (
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

func TestGettingStartedProgramCompiles(t *testing.T) {
	root := repoRoot(t)
	var program string
	for _, name := range []string{"getting-started.md", "getting-started.zh-CN.md"} {
		path := filepath.Join(root, "docs", name)
		body, err := readRepositoryFile(path)
		if err != nil {
			t.Fatal(err)
		}
		got := codeFences(string(body), "go")
		if got == "" {
			t.Fatalf("%s has no Go program", relative(root, path))
		}
		if program == "" {
			program = got
		} else if got != program {
			t.Fatalf("%s and the English tutorial contain different programs", relative(root, path))
		}
	}

	dir := t.TempDir()
	mod := "module example.com/oolong-getting-started\n\n" +
		"go 1.25.0\n\n" +
		"require github.com/Tangerg/oolong/core v0.0.0\n\n" +
		"replace github.com/Tangerg/oolong/core => " + strconv.Quote(filepath.ToSlash(filepath.Join(root, "core"))) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(program), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "go", "test", "-mod=mod", ".")
	command.Dir = dir
	command.Env = append(os.Environ(), "GOWORK=off")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("getting-started program does not compile: %v\n%s", err, output)
	}
}

// TestLearningPathStaysRunnableAndOrdered ties every teaching step to a tested
// vertical slice. A guide may explain less than its example contains, but it must
// never become an isolated recipe with no executable proof behind it.
func TestLearningPathStaysRunnableAndOrdered(t *testing.T) {
	root := repoRoot(t)
	index, err := readRepositoryFile(filepath.Join(root, "docs", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	indexText := string(index)
	readme, err := readRepositoryFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	readmeText := string(readme)

	steps := []struct {
		guide   string
		example string
	}{
		{"getting-started.md", "hello"},
		{"components.md", "picker"},
		{"content.md", "content"},
		{"streaming.md", "streaming"},
		{"agent.md", "agent"},
	}
	previous := -1
	for _, step := range steps {
		needle := "](" + step.guide + ")"
		at := strings.Index(indexText, needle)
		switch {
		case at < 0:
			t.Errorf("docs/README.md does not list %s", step.guide)
		case at <= previous:
			t.Errorf("docs/README.md lists %s outside the learning order", step.guide)
		default:
			previous = at
		}

		if !strings.Contains(readmeText, "](docs/"+step.guide+")") {
			t.Errorf("README.md does not expose docs/%s", step.guide)
		}
		assertGuideExample(t, root, step.guide, step.example)
	}
}

func assertGuideExample(t *testing.T, root, guide, example string) {
	t.Helper()
	for _, name := range []string{guide, strings.TrimSuffix(guide, ".md") + ".zh-CN.md"} {
		body, err := readRepositoryFile(filepath.Join(root, "docs", name))
		if err != nil {
			t.Error(err)
			continue
		}
		if !strings.Contains(string(body), "](../examples/"+example+")") {
			t.Errorf("docs/%s does not point to examples/%s", name, example)
		}
	}
	for _, name := range []string{"main.go", "main_test.go"} {
		path := filepath.Join(root, "examples", example, name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("docs/%s relies on an untested example: %s", guide, relative(root, path))
		}
	}
}

func TestTranslatedDocumentationStaysPaired(t *testing.T) {
	root := repoRoot(t)
	docs := filepath.Join(root, "docs")
	entries, err := os.ReadDir(docs)
	if err != nil {
		t.Fatal(err)
	}
	pairs := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".zh-CN.md") {
			continue
		}
		pairs++
		englishName := strings.TrimSuffix(name, ".zh-CN.md") + ".md"
		english, err := readRepositoryFile(filepath.Join(docs, englishName))
		if err != nil {
			t.Errorf("%s has no English source %s", name, englishName)
			continue
		}
		chinese, err := readRepositoryFile(filepath.Join(docs, name))
		if err != nil {
			t.Error(err)
			continue
		}
		if !strings.Contains(string(english), "]("+name+")") {
			t.Errorf("%s does not link to its Chinese translation", englishName)
		}
		if !strings.Contains(string(chinese), "]("+englishName+")") {
			t.Errorf("%s does not link to its English source", name)
		}
		englishHeadings := len(markdownHeading.FindAll(english, -1))
		chineseHeadings := len(markdownHeading.FindAll(chinese, -1))
		if englishHeadings != chineseHeadings {
			t.Errorf("%s has %d headings; %s has %d", englishName, englishHeadings, name, chineseHeadings)
		}
	}
	if pairs == 0 {
		t.Fatal("no translated documentation pairs found")
	}
}

var (
	inlineMarkdownLink = regexp.MustCompile(`!?\[[^\]]*\]\(\s*(?:<([^>\n]+)>|([^\s)\n]+))`)
	referenceLink      = regexp.MustCompile(`(?m)^\[[^\]]+\]:\s*(?:<([^>\n]+)>|(\S+))`)
	markdownHeading    = regexp.MustCompile(`(?m)^ {0,3}#{1,6}[ \t]+(.+?)[ \t]*#*[ \t]*$`)
	headingLink        = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
	headingHTML        = regexp.MustCompile(`<[^>]*>`)
)

// TestEveryLocalDocumentationLinkResolves makes the documentation graph an
// executable promise. GitHub can render a missing relative path or stale heading
// anchor only as a dead end, and neither go test nor markdownlint observes it.
func TestEveryLocalDocumentationLinkResolves(t *testing.T) {
	root := repoRoot(t)
	checked := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skipped(entry.Name(), path == root) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		body, err := readRepositoryFile(path)
		if err != nil {
			return err
		}
		content := maskFencedCode(string(body))
		for _, pattern := range []*regexp.Regexp{inlineMarkdownLink, referenceLink} {
			for _, match := range pattern.FindAllStringSubmatchIndex(content, -1) {
				destination := captured(content, match, 1)
				if destination == "" {
					destination = captured(content, match, 2)
				}
				line := strings.Count(content[:match[0]], "\n") + 1
				checked++
				checkMarkdownLink(t, root, path, line, destination)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk documentation: %v", err)
	}
	if checked == 0 {
		t.Fatal("no documentation links checked, so the link gate proves nothing")
	}
}

func checkMarkdownLink(t *testing.T, root, source string, line int, destination string) {
	t.Helper()
	parsed, err := url.Parse(destination)
	if err != nil {
		t.Errorf("%s:%d has an invalid link %q: %v", relative(root, source), line, destination, err)
		return
	}
	if parsed.IsAbs() || parsed.Host != "" || strings.HasPrefix(destination, "//") {
		return
	}

	linked := source
	if parsed.Path != "" {
		path, unescapeErr := url.PathUnescape(parsed.Path)
		if unescapeErr != nil {
			t.Errorf("%s:%d has an invalid escaped path %q: %v", relative(root, source), line, destination, unescapeErr)
			return
		}
		linked = filepath.Clean(filepath.Join(filepath.Dir(source), filepath.FromSlash(path)))
	}
	info, err := os.Stat(linked)
	if err != nil {
		t.Errorf("%s:%d links to missing %q", relative(root, source), line, destination)
		return
	}
	if parsed.Fragment == "" {
		return
	}
	if info.IsDir() {
		linked = filepath.Join(linked, "README.md")
		if _, statErr := os.Stat(linked); statErr != nil {
			t.Errorf("%s:%d links to anchor %q in a directory with no README", relative(root, source), line, destination)
			return
		}
	}
	if filepath.Ext(linked) != ".md" {
		t.Errorf("%s:%d links to anchor %q in a non-Markdown file", relative(root, source), line, destination)
		return
	}
	anchors, err := markdownAnchors(linked)
	if err != nil {
		t.Errorf("%s:%d cannot read %q: %v", relative(root, source), line, destination, err)
		return
	}
	if !anchors[parsed.Fragment] {
		t.Errorf("%s:%d links to missing anchor %q", relative(root, source), line, destination)
	}
}

func markdownAnchors(path string) (map[string]bool, error) {
	body, err := readRepositoryFile(path)
	if err != nil {
		return nil, err
	}
	anchors := make(map[string]bool)
	seen := make(map[string]int)
	for _, match := range markdownHeading.FindAllStringSubmatch(maskFencedCode(string(body)), -1) {
		base := githubSlug(match[1])
		slug := base
		if duplicate := seen[base]; duplicate > 0 {
			slug += "-" + strconv.Itoa(duplicate)
		}
		seen[base]++
		anchors[slug] = true
	}
	return anchors, nil
}

func githubSlug(heading string) string {
	heading = headingLink.ReplaceAllString(heading, "$1")
	heading = headingHTML.ReplaceAllString(heading, "")
	heading = strings.ReplaceAll(heading, "`", "")
	var slug strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r), r == '-', r == '_':
			slug.WriteRune(r)
		case unicode.IsSpace(r):
			slug.WriteByte('-')
		}
	}
	return slug.String()
}

func maskFencedCode(content string) string {
	var masked strings.Builder
	inside := false
	for _, line := range strings.SplitAfter(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inside = !inside
			masked.WriteString(strings.Repeat(" ", len(strings.TrimSuffix(line, "\n"))))
			if strings.HasSuffix(line, "\n") {
				masked.WriteByte('\n')
			}
			continue
		}
		if inside {
			masked.WriteString(strings.Repeat(" ", len(strings.TrimSuffix(line, "\n"))))
			if strings.HasSuffix(line, "\n") {
				masked.WriteByte('\n')
			}
			continue
		}
		masked.WriteString(line)
	}
	return masked.String()
}

func codeFences(content, language string) string {
	var code strings.Builder
	inside := false
	for _, line := range strings.SplitAfter(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(trimmed, "```"); ok {
			if inside {
				inside = false
				code.WriteByte('\n')
			} else if strings.TrimSpace(after) == language {
				inside = true
			}
			continue
		}
		if inside {
			code.WriteString(line)
		}
	}
	return code.String()
}

func captured(content string, match []int, group int) string {
	at := group * 2
	if at+1 >= len(match) || match[at] < 0 {
		return ""
	}
	return content[match[at]:match[at+1]]
}

func relative(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

func readRepositoryFile(path string) ([]byte, error) {
	// Paths come from fixed documentation names or a walk rooted at repoRoot.
	//nolint:gosec // G304: the test deliberately validates repository-owned files.
	return os.ReadFile(path)
}
