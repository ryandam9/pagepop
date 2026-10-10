package builder

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"pagepop/internal/logutil"
)

func TestExtractMeta(t *testing.T) {
	src := `# My Title
- Created - 2024/05/18
- Tags - go, testing, web
- Description - A test post for Pagepop

This is the body of the post.
`
	meta, body := extractMeta(src, "my-post.md")

	expectedDate, _ := time.Parse("2006/01/02", "2024/05/18")
	if meta.Title != "My Title" {
		t.Errorf("expected Title 'My Title', got '%s'", meta.Title)
	}
	if !meta.Date.Equal(expectedDate) {
		t.Errorf("expected Date %v, got %v", expectedDate, meta.Date)
	}
	expectedTags := []string{"go", "testing", "web"}
	if !reflect.DeepEqual(meta.Tags, expectedTags) {
		t.Errorf("expected Tags %v, got %v", expectedTags, meta.Tags)
	}
	if meta.Description != "A test post for Pagepop" {
		t.Errorf("expected Description 'A test post for Pagepop', got '%s'", meta.Description)
	}
	if body != "This is the body of the post." {
		t.Errorf("expected body 'This is the body of the post.', got '%s'", body)
	}
	if meta.Slug != "my-post" {
		t.Errorf("expected slug 'my-post', got '%s'", meta.Slug)
	}
}

func TestExtractMetaFilenameDate(t *testing.T) {
	tests := []struct {
		name, src, filename, wantDate, wantSlug string
	}{
		{"no created line", "# T\n\nbody", "2026.10.09-my-post.md", "2026/10/09", "my-post"},
		{"created line wins", "# T\n- Created - 2024/05/18\n\nbody", "2026.10.09-my-post.md", "2024/05/18", "my-post"},
		{"date only filename", "# T\n\nbody", "2026.10.09.md", "2026/10/09", "20261009"},
		{"no date anywhere", "# T\n\nbody", "my-post.md", "1900/01/01", "my-post"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, _ := extractMeta(tt.src, tt.filename)
			if got := meta.Date.Format("2006/01/02"); got != tt.wantDate {
				t.Errorf("date = %s, want %s", got, tt.wantDate)
			}
			if meta.Slug != tt.wantSlug {
				t.Errorf("slug = %q, want %q", meta.Slug, tt.wantSlug)
			}
		})
	}
}

func TestFixImagePaths(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    `<img src="images/bear.png">`,
			expected: `<img src="bear.png">`,
		},
		{
			input:    `<img src="./images/bear.png">`,
			expected: `<img src="bear.png">`,
		},
		{
			input:    `<img src="../images/bear.png">`,
			expected: `<img src="bear.png">`,
		},
		{
			input:    `<img class="foo" src="bear.png">`,
			expected: `<img class="foo" src="bear.png">`,
		},
	}

	for _, tt := range tests {
		got := fixImagePaths(tt.input)
		if got != tt.expected {
			t.Errorf("fixImagePaths(%s) = %s; want %s", tt.input, got, tt.expected)
		}
	}
}

func TestSite(t *testing.T) {
	tempDir := t.TempDir()

	mdPath := filepath.Join(tempDir, "post.md")
	mdContent := `# Hello World
- Created - 2024/01/01
- Description - A dummy post
- Tags - dummy, test

This is a **bold** test.`
	if err := os.WriteFile(mdPath, []byte(mdContent), 0644); err != nil {
		t.Fatalf("failed to write md: %v", err)
	}

	imgDir := filepath.Join(tempDir, "images")
	if err := os.MkdirAll(imgDir, 0755); err != nil {
		t.Fatalf("failed to create images dir: %v", err)
	}
	imgPath := filepath.Join(imgDir, "dummy.png")
	if err := os.WriteFile(imgPath, []byte("fakeimage"), 0644); err != nil {
		t.Fatalf("failed to write image: %v", err)
	}

	f, _ := os.OpenFile(mdPath, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString("\n\n![dummy image](images/dummy.png)")
	f.Close()

	configPath := filepath.Join(tempDir, "md_files.yml")
	configContent := fmt.Sprintf("site:\n  title: My E2E Site\n  base_url: https://example.com\nmarkdown_files:\n  - file: %s", filepath.ToSlash(mdPath))
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	outDir := filepath.Join(tempDir, "blog")

	if err := Site(outDir, configPath, true, false, false, logutil.NewDiscard()); err != nil {
		t.Fatalf("Site() failed: %v", err)
	}

	// With embed-styles off, posts must link style.css via a path relative to
	// their own directory so the blog works when served from a subdirectory.
	noEmbedDir := filepath.Join(tempDir, "blog-noembed")
	if err := Site(noEmbedDir, configPath, false, false, false, logutil.NewDiscard()); err != nil {
		t.Fatalf("Site() (no embed) failed: %v", err)
	}
	noEmbedHTML, err := os.ReadFile(filepath.Join(noEmbedDir, "2024/01/01", "post", "index.html"))
	if err != nil {
		t.Fatalf("reading no-embed post: %v", err)
	}
	cssQuery := cssVersionQuery(buildCSS())
	if !strings.Contains(string(noEmbedHTML), `href="../../../../style.css`+cssQuery+`"`) {
		t.Errorf("post does not link style.css with a relative, versioned path; got:\n%s", noEmbedHTML)
	}
	if strings.Contains(string(noEmbedHTML), `href="/style.css`) {
		t.Errorf("post still links style.css with a root-absolute path")
	}
	listingHTML, err := os.ReadFile(filepath.Join(noEmbedDir, "blog_entries.html"))
	if err != nil {
		t.Fatalf("reading listing: %v", err)
	}
	if !strings.Contains(string(listingHTML), `href="style.css`+cssQuery+`"`) {
		t.Errorf("listing does not link a versioned style.css")
	}

	// A post that is up to date but links an older stylesheet version must be
	// rebuilt, or browsers keep serving it the stale cached CSS.
	postPath := filepath.Join(noEmbedDir, "2024/01/01", "post", "index.html")
	stale := strings.Replace(string(noEmbedHTML), cssQuery, "?v=00000000", 1)
	if err := os.WriteFile(postPath, []byte(stale), 0644); err != nil {
		t.Fatalf("writing stale post: %v", err)
	}
	if err := Site(noEmbedDir, configPath, false, false, false, logutil.NewDiscard()); err != nil {
		t.Fatalf("Site() (rebuild) failed: %v", err)
	}
	rebuilt, _ := os.ReadFile(postPath)
	if !strings.Contains(string(rebuilt), cssQuery) {
		t.Errorf("post linking an old stylesheet version was not rebuilt")
	}

	if _, err := os.Stat(filepath.Join(outDir, "blog_entries.html")); os.IsNotExist(err) {
		t.Errorf("listing blog_entries.html not generated")
	}
	if _, err := os.Stat(filepath.Join(outDir, "style.css")); os.IsNotExist(err) {
		t.Errorf("root style.css not generated")
	}
	if _, err := os.Stat(filepath.Join(outDir, "feed.xml")); os.IsNotExist(err) {
		t.Errorf("feed.xml not generated")
	}
	if _, err := os.Stat(filepath.Join(outDir, "sitemap.xml")); os.IsNotExist(err) {
		t.Errorf("sitemap.xml not generated")
	}

	postDir := filepath.Join(outDir, "2024/01/01", "post")
	if _, err := os.Stat(filepath.Join(postDir, "index.html")); os.IsNotExist(err) {
		t.Errorf("post index.html not generated")
	}
	if _, err := os.Stat(filepath.Join(postDir, "style.css")); os.IsNotExist(err) {
		t.Errorf("post style.css not generated (embed-styles)")
	}
	if _, err := os.Stat(filepath.Join(postDir, "dummy.png")); os.IsNotExist(err) {
		t.Errorf("image not copied to post dir")
	}
}

func TestTOCPosition(t *testing.T) {
	body := template.HTML("<div class=\"post-body\"></div>")
	toc := template.HTML(`<nav class="toc"></nav>`)
	meta := postMeta{Title: "T", Date: time.Now()}

	side, err := wrapPost(meta, body, toc, "style.css", false, SiteConfig{})
	if err != nil {
		t.Fatalf("wrapPost (side) failed: %v", err)
	}
	if !strings.Contains(side, "post-layout toc-side") {
		t.Errorf("default layout should be toc-side; got:\n%s", side)
	}

	top, err := wrapPost(meta, body, toc, "style.css", true, SiteConfig{})
	if err != nil {
		t.Fatalf("wrapPost (top) failed: %v", err)
	}
	if !strings.Contains(top, "post-layout toc-top") {
		t.Errorf("--toc-top should render toc-top layout; got:\n%s", top)
	}
}

func TestTransformCallouts(t *testing.T) {
	cases := []struct {
		name      string
		md        string
		wantClass string
	}{
		{"note", "> [!NOTE]\n> A note.\n", "callout callout-note"},
		{"tip", "> [!TIP]\n> A tip.\n", "callout callout-tip"},
		{"important", "> [!IMPORTANT]\n> Important info.\n", "callout callout-important"},
		{"warning", "> [!WARNING]\n> A warning.\n", "callout callout-warning"},
		{"caution", "> [!CAUTION]\n> Be careful.\n", "callout callout-caution"},
		{"lowercase", "> [!warning]\n> lower.\n", "callout callout-warning"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := string(renderMarkdown(tc.md))
			if !strings.Contains(out, `class="`+tc.wantClass+`"`) {
				t.Errorf("expected callout class %q, got: %s", tc.wantClass, out)
			}
			if strings.Contains(out, "[!") {
				t.Errorf("callout marker not stripped, got: %s", out)
			}
		})
	}
}

func TestEmbedTweets(t *testing.T) {
	cases := []struct {
		name  string
		md    string
		embed string // expected canonical URL, "" for no embed
	}{
		{"bare x.com", "https://x.com/jack/status/20\n", "https://twitter.com/jack/status/20"},
		{"bare twitter.com with query", "https://twitter.com/jack/status/20?s=21&t=abc\n", "https://twitter.com/jack/status/20"},
		{"angle autolink", "<https://www.x.com/jack/status/20>\n", "https://twitter.com/jack/status/20"},
		{"markdown link", "[a post](https://x.com/jack/status/20)\n", "https://twitter.com/jack/status/20"},
		{"inline in sentence", "See https://x.com/jack/status/20 for details.\n", ""},
		{"profile link", "https://x.com/jack\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := string(renderMarkdown(tc.md))
			if tc.embed == "" {
				if hasTweets(out) {
					t.Errorf("did not expect an embed, got: %s", out)
				}
				return
			}
			if !hasTweets(out) || !strings.Contains(out, `href="`+tc.embed+`"`) {
				t.Errorf("expected embed of %s, got: %s", tc.embed, out)
			}
		})
	}

	meta := postMeta{Title: "T", Date: time.Now()}
	with, _ := wrapPost(meta, renderMarkdown("https://x.com/jack/status/20\n"), "", "style.css", false, SiteConfig{})
	if !strings.Contains(with, "platform.twitter.com/widgets.js") {
		t.Errorf("post with an embedded tweet should load widgets.js")
	}
	without, _ := wrapPost(meta, renderMarkdown("Hello\n"), "", "style.css", false, SiteConfig{})
	if strings.Contains(without, "platform.twitter.com") {
		t.Errorf("post without tweets must not load widgets.js")
	}
}

func TestHTMLPages(t *testing.T) {
	tempDir := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(tempDir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	write("standalone.html", `<html><head><title>Stand &amp; Alone</title><meta name="description" content="From the page"></head><body>hi</body></html>`)
	write("demo/app.html", `<html><head><title>ignored</title><script src="js/app.js"></script></head></html>`)
	write("demo/js/app.js", `console.log(1)`)
	write("demo/.git/config", `secret`)

	config := fmt.Sprintf(`site:
  title: T
html_pages:
  - file: %s
    created: 2025/02/03
  - dir: %s
    index: app.html
    title: My Demo
    description: Interactive demo
    tags: [js, demo]
    created: 2025/03/04
`, filepath.Join(tempDir, "standalone.html"), filepath.Join(tempDir, "demo"))
	write("cfg.yml", config)

	out := filepath.Join(tempDir, "blog")
	if err := Site(out, filepath.Join(tempDir, "cfg.yml"), false, false, false, logutil.NewDiscard()); err != nil {
		t.Fatalf("Site() failed: %v", err)
	}

	for _, rel := range []string{
		"2025/02/03/standalone/index.html",
		"2025/03/04/demo/app.html",
		"2025/03/04/demo/index.html",
		"2025/03/04/demo/js/app.js",
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Errorf("expected %s: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "2025/03/04/demo/.git")); err == nil {
		t.Errorf("dot-directories must not be published")
	}

	listing, err := os.ReadFile(filepath.Join(out, "blog_entries.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`href='2025/02/03/standalone/index.html'`,
		`Stand &amp; Alone`,
		`From the page`,
		`href='2025/03/04/demo/index.html'`,
		`My Demo`,
		`Interactive demo`,
	} {
		if !strings.Contains(string(listing), want) {
			t.Errorf("listing missing %q", want)
		}
	}
	// Tag pages live two levels down, so post links must climb back to the root.
	tagListing, err := os.ReadFile(filepath.Join(out, "tags", "demo", "blog_entries.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tagListing), `href='../../2025/03/04/demo/index.html'`) {
		t.Errorf("tag listing post link not relative to site root")
	}
	// Newest first.
	if strings.Index(string(listing), "My Demo") > strings.Index(string(listing), "Stand &amp; Alone") {
		t.Errorf("listing not sorted newest first")
	}
}

func TestMarkdownDir(t *testing.T) {
	tempDir := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(tempDir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	posts := filepath.Join(tempDir, "posts")
	write("posts/2025.01.02-first.md", "# First\n\nbody")
	write("posts/second.MD", "# Second\n- Created - 2025/01/03\n\nbody")
	write("posts/notes.txt", "not markdown")
	write("posts/.hidden.md", "# Hidden")
	write("posts/drafts/draft.md", "# Draft")

	// The second file is also listed on its own; it must be built only once.
	config := fmt.Sprintf("markdown_files:\n  - dir: %s\n  - file: %s\n",
		filepath.ToSlash(posts), filepath.ToSlash(filepath.Join(posts, "second.MD")))
	write("cfg.yml", config)

	got := markdownFiles([]mdEntry{{Dir: posts}, {File: filepath.Join(posts, "second.MD")}}, logutil.NewDiscard())
	want := []string{filepath.Join(posts, "2025.01.02-first.md"), filepath.Join(posts, "second.MD")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("markdownFiles() = %v, want %v", got, want)
	}

	out := filepath.Join(tempDir, "blog")
	if err := Site(out, filepath.Join(tempDir, "cfg.yml"), false, false, false, logutil.NewDiscard()); err != nil {
		t.Fatalf("Site() failed: %v", err)
	}
	for _, rel := range []string{"2025/01/02/first/index.html", "2025/01/03/second/index.html"} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Errorf("expected %s: %v", rel, err)
		}
	}
	for _, rel := range []string{"1900/01/01/hidden", "1900/01/01/draft"} {
		if _, err := os.Stat(filepath.Join(out, rel)); err == nil {
			t.Errorf("%s should not have been built", rel)
		}
	}
}
