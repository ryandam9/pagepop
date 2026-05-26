package builder

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

	if err := Site(outDir, configPath, true, false, logutil.NewDiscard()); err != nil {
		t.Fatalf("Site() failed: %v", err)
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
