package arch

import (
	"errors"
	"fmt"
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
	for _, name := range []string{"getting-started.md", filepath.Join("zh", "getting-started.md")} {
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
	link := "](https://github.com/Tangerg/oolong/tree/main/examples/" + example + ")"
	for _, name := range []string{guide, filepath.Join("zh", guide)} {
		body, err := readRepositoryFile(filepath.Join(root, "docs", name))
		if err != nil {
			t.Error(err)
			continue
		}
		if !strings.Contains(string(body), link) {
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
		if entry.IsDir() || !strings.HasSuffix(name, ".md") {
			continue
		}
		pairs++
		englishName := name
		english, err := readRepositoryFile(filepath.Join(docs, englishName))
		if err != nil {
			t.Error(err)
			continue
		}
		chineseName := filepath.Join("zh", name)
		chinese, err := readRepositoryFile(filepath.Join(docs, chineseName))
		if err != nil {
			t.Errorf("%s has no Chinese translation %s", englishName, filepath.ToSlash(chineseName))
			continue
		}
		englishLink := "](zh/" + name + ")"
		chineseLink := "](../" + name + ")"
		if name == "README.md" {
			englishLink = "](/zh/)"
			chineseLink = "](/)"
		}
		if !strings.Contains(string(english), englishLink) {
			t.Errorf("%s does not link to its Chinese translation", englishName)
		}
		if !strings.Contains(string(chinese), chineseLink) {
			t.Errorf("%s does not link to its English source", filepath.ToSlash(chineseName))
		}
		assertDocumentationPurpose(t, filepath.ToSlash(filepath.Join("docs", englishName)), english)
		assertDocumentationPurpose(t, filepath.ToSlash(filepath.Join("docs", chineseName)), chinese)
		englishHeadings := len(markdownHeading.FindAll(english, -1))
		chineseHeadings := len(markdownHeading.FindAll(chinese, -1))
		if englishHeadings != chineseHeadings {
			t.Errorf("%s has %d headings; %s has %d", englishName, englishHeadings, filepath.ToSlash(chineseName), chineseHeadings)
		}
	}
	if pairs == 0 {
		t.Fatal("no translated documentation pairs found")
	}
}

func assertDocumentationPurpose(t *testing.T, name string, body []byte) {
	t.Helper()
	fields, ok := documentationFrontMatter(string(body))
	if !ok {
		t.Errorf("%s must begin with YAML frontmatter", name)
		return
	}
	for _, field := range []string{"title", "description", "contentType"} {
		if fields[field] == "" {
			t.Errorf("%s frontmatter has no %s", name, field)
		}
	}
	allowed := map[string]bool{
		"Conceptual":      true,
		"How-to":          true,
		"Landing":         true,
		"Reference":       true,
		"Troubleshooting": true,
		"Tutorial":        true,
	}
	if kind := fields["contentType"]; kind != "" && !allowed[kind] {
		t.Errorf("%s has unknown contentType %q", name, kind)
	}
}

func documentationFrontMatter(body string) (map[string]string, bool) {
	rest, ok := strings.CutPrefix(body, "---\n")
	if !ok {
		return nil, false
	}
	header, _, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		return nil, false
	}
	fields := make(map[string]string)
	for line := range strings.SplitSeq(header, "\n") {
		key, value, found := strings.Cut(line, ":")
		if found {
			fields[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return fields, true
}

var (
	inlineMarkdownLink = regexp.MustCompile(`!?\[[^\]]*\]\(\s*(?:<([^>\n]+)>|([^\s)\n]+))`)
	referenceLink      = regexp.MustCompile(`(?m)^\[[^\]]+\]:\s*(?:<([^>\n]+)>|(\S+))`)
	markdownHeading    = regexp.MustCompile(`(?m)^ {0,3}#{1,6}[ \t]+(.+?)[ \t]*#*[ \t]*$`)
	headingLink        = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
	headingHTML        = regexp.MustCompile(`<[^>]*>`)
	repositoryLink     = regexp.MustCompile(`^/Tangerg/oolong/(?:blob|tree)/main/(.+)$`)
)

// TestEveryRepositoryDocumentationLinkResolves makes the documentation graph an
// executable promise. Relative links and same-repository GitHub URLs both resolve
// against this checkout; otherwise a renamed source file becomes a quiet dead end
// that neither go test nor markdownlint observes.
func TestEveryRepositoryDocumentationLinkResolves(t *testing.T) {
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
				if checkMarkdownLink(t, root, path, line, destination) {
					checked++
				}
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

func checkMarkdownLink(t *testing.T, root, source string, line int, destination string) bool {
	t.Helper()
	checked, err := validateMarkdownLink(root, source, destination)
	if err != nil {
		t.Errorf("%s:%d %v", relative(root, source), line, err)
	}
	return checked
}

func validateMarkdownLink(root, source, destination string) (bool, error) {
	parsed, err := url.Parse(destination)
	if err != nil {
		return true, fmt.Errorf("has an invalid link %q: %w", destination, err)
	}
	linked, local, err := documentationLinkTarget(root, source, parsed)
	if err != nil {
		return true, fmt.Errorf("has an invalid link %q: %w", destination, err)
	}
	if !local {
		return false, nil
	}
	info, err := os.Stat(linked)
	if err != nil {
		return true, fmt.Errorf("links to missing %q", destination)
	}
	if parsed.Fragment == "" {
		return true, nil
	}
	if info.IsDir() {
		linked = filepath.Join(linked, "README.md")
		if _, statErr := os.Stat(linked); statErr != nil {
			return true, fmt.Errorf("links to anchor %q in a directory with no README", destination)
		}
	}
	if filepath.Ext(linked) != ".md" {
		return true, fmt.Errorf("links to anchor %q in a non-Markdown file", destination)
	}
	anchors, err := markdownAnchors(linked)
	if err != nil {
		return true, fmt.Errorf("cannot read %q: %w", destination, err)
	}
	if !anchors[parsed.Fragment] {
		return true, fmt.Errorf("links to missing anchor %q", destination)
	}
	return true, nil
}

// documentationLinkTarget maps links whose truth lives in this checkout back to
// the checkout. Ordinary web links remain the web's responsibility, but spelling
// a repository file as a GitHub URL must not make it invisible to this gate.
func documentationLinkTarget(root, source string, parsed *url.URL) (string, bool, error) {
	if parsed.IsAbs() || parsed.Host != "" {
		if parsed.Scheme != "https" || !strings.EqualFold(parsed.Host, "github.com") {
			return "", false, nil
		}
		match := repositoryLink.FindStringSubmatch(parsed.EscapedPath())
		if match == nil {
			return "", false, nil
		}
		path, err := url.PathUnescape(match[1])
		if err != nil {
			return "", false, err
		}
		linked := filepath.Clean(filepath.Join(root, filepath.FromSlash(path)))
		rel, err := filepath.Rel(root, linked)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", false, errors.New("repository path leaves the checkout")
		}
		return linked, true, nil
	}

	linked := source
	if parsed.Path != "" {
		path, err := url.PathUnescape(parsed.EscapedPath())
		if err != nil {
			return "", false, err
		}
		linked = filepath.Clean(filepath.Join(filepath.Dir(source), filepath.FromSlash(path)))
	}
	return linked, true, nil
}

func TestRepositoryMarkdownLinksRemainCheckoutContracts(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "docs", "guide.md")
	target := filepath.Join(root, "core", "grid", "inline.go")
	for _, dir := range []string{filepath.Dir(source), filepath.Dir(target)} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(source, []byte("# Guide\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("package grid\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		destination string
		wantChecked bool
		wantErr     bool
	}{
		{name: "repository file", destination: "https://github.com/Tangerg/oolong/blob/main/core/grid/inline.go", wantChecked: true},
		{name: "missing repository file", destination: "https://github.com/Tangerg/oolong/blob/main/core/grid/missing.go", wantChecked: true, wantErr: true},
		{name: "escaped checkout", destination: "https://github.com/Tangerg/oolong/blob/main/%2e%2e/outside.go", wantChecked: true, wantErr: true},
		{name: "external website", destination: "https://example.com/not-checked-here"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checked, err := validateMarkdownLink(root, source, test.destination)
			if checked != test.wantChecked {
				t.Errorf("validateMarkdownLink() checked = %v, want %v", checked, test.wantChecked)
			}
			if (err != nil) != test.wantErr {
				t.Fatalf("validateMarkdownLink() error = %v, wantErr %v", err, test.wantErr)
			}
		})
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
