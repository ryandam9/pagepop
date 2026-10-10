package builder

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"pagepop/internal/logutil"

	"github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"gopkg.in/yaml.v3"
)

type SiteConfig struct {
	Title       string `yaml:"title"`
	Author      string `yaml:"author"`
	BaseURL     string `yaml:"base_url"`
	Description string `yaml:"description"`
	Language    string `yaml:"language"`
}

// mdEntry names one Markdown file, or a directory whose *.md files are all
// published.
type mdEntry struct {
	File string `yaml:"file"`
	Dir  string `yaml:"dir"`
}

type mdConfig struct {
	Site          SiteConfig  `yaml:"site"`
	MarkdownFiles []mdEntry   `yaml:"markdown_files"`
	HTMLPages     []htmlEntry `yaml:"html_pages"`
}

type postMeta struct {
	Title       string
	Slug        string
	Date        time.Time
	Description string
	Tags        []string
}

type post struct {
	Meta postMeta
	Body template.HTML
}

//go:embed style.css
var defaultCSS string

//go:embed post.html
var postTemplate string

//go:embed listing.html
var listingTemplate string

var (
	reDate        = regexp.MustCompile(`(\d{4}/\d{2}/\d{2})`)
	reFileDate    = regexp.MustCompile(`^(\d{4}\.\d{2}\.\d{2})[-_. ]*`)
	reTags        = regexp.MustCompile(`(?i)^-\s*tags\s*-\s*(.+)`)
	reDescription = regexp.MustCompile(`(?i)^-\s*description\s*-\s*(.+)`)
	reCreated     = regexp.MustCompile(`(?i)^-\s*created\s*-\s*(.+)`)
	reImgSrc      = regexp.MustCompile(`<img\s+[^>]*src="([^"]+)"`)
	reFixImgPath  = regexp.MustCompile(`(<img\s+[^>]*src=")(?:\.\./|\./)?(?:images/)?([^"]+")`)
	reSlugClean   = regexp.MustCompile(`[^a-z0-9-]`)
	reHeadingTag  = regexp.MustCompile(`(?s)<h([2-4])>(.*?)</h[2-4]>`)
	reStripTags   = regexp.MustCompile(`<[^>]+>`)
)

// markdownFiles expands the markdown_files entries into a list of files. A dir
// entry contributes the *.md files directly inside it, in name order; hidden
// files and subdirectories are skipped. A file reached through more than one
// entry is listed once.
func markdownFiles(entries []mdEntry, log *logutil.Logger) []string {
	var files []string
	seen := map[string]bool{}
	add := func(f string) {
		key := filepath.Clean(f)
		if abs, err := filepath.Abs(key); err == nil {
			key = abs
		}
		if seen[key] {
			return
		}
		seen[key] = true
		files = append(files, f)
	}

	for _, e := range entries {
		if (e.File == "") == (e.Dir == "") {
			log.Warn("markdown_files entry needs exactly one of file or dir")
			continue
		}
		if e.File != "" {
			add(e.File)
			continue
		}
		dirEntries, err := os.ReadDir(e.Dir)
		if err != nil {
			log.Warn("reading markdown dir %s: %v", e.Dir, err)
			continue
		}
		found := 0
		for _, d := range dirEntries {
			name := d.Name()
			if d.IsDir() || strings.HasPrefix(name, ".") || !strings.EqualFold(filepath.Ext(name), ".md") {
				continue
			}
			add(filepath.Join(e.Dir, name))
			found++
		}
		if found == 0 {
			log.Warn("markdown dir %s contains no .md files", e.Dir)
		}
	}
	return files
}

// Site builds the blog directory from a YAML config file.
//
// Output layout:
//
//	outputDir/
//	  style.css
//	  index.html         (post listing)
//	  <slug>/
//	    index.html
//	    <assets>
func Site(outputDir, configPath string, embedStyles, clean, tocTop bool, log *logutil.Logger) error {
	if clean {
		if err := os.RemoveAll(outputDir); err != nil {
			return fmt.Errorf("cleaning output dir: %w", err)
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	var cfg mdConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}

	if cfg.Site.Title == "" {
		cfg.Site.Title = "Blog"
	}
	if cfg.Site.Language == "" {
		cfg.Site.Language = "en"
	}
	// Remove trailing slash from base url
	cfg.Site.BaseURL = strings.TrimSuffix(cfg.Site.BaseURL, "/")

	cssBytes := buildCSS()
	cssQuery := cssVersionQuery(cssBytes)

	var posts []post

	for _, file := range markdownFiles(cfg.MarkdownFiles, log) {
		p, err := processFile(file, outputDir, cssBytes, cssQuery, embedStyles, tocTop, cfg.Site, log)
		if err != nil {
			log.Warn("processing file %s: %v", file, err)
			continue
		}
		posts = append(posts, p)
	}

	for _, entry := range cfg.HTMLPages {
		p, err := processHTMLPage(entry, outputDir, log)
		if err != nil {
			log.Warn("processing html page %s%s: %v", entry.File, entry.Dir, err)
			continue
		}
		posts = append(posts, p)
	}

	// Two entries resolving to the same date and slug would overwrite each
	// other's output; say so rather than silently publishing only one.
	seen := map[string]bool{}
	for _, p := range posts {
		key := p.Meta.Date.Format("2006/01/02") + "/" + p.Meta.Slug
		if seen[key] {
			log.Warn("two entries share the output path %s; one overwrites the other", key)
		}
		seen[key] = true
	}

	sort.Slice(posts, func(i, j int) bool {
		return posts[i].Meta.Date.After(posts[j].Meta.Date)
	})

	if err := writeBlogListing(outputDir, "", posts, cfg.Site, cssQuery); err != nil {
		return fmt.Errorf("writing blog listing: %w", err)
	}

	if err := writeTagIndexes(outputDir, posts, cfg.Site, cssBytes, cssQuery); err != nil {
		return fmt.Errorf("writing tag indexes: %w", err)
	}

	if cfg.Site.BaseURL != "" {
		if err := writeRSS(outputDir, posts, cfg.Site); err != nil {
			return fmt.Errorf("writing rss: %w", err)
		}
		if err := writeSitemap(outputDir, posts, cfg.Site); err != nil {
			return fmt.Errorf("writing sitemap: %w", err)
		}
	}

	if err := copyStatic(outputDir, cssBytes); err != nil {
		return fmt.Errorf("copying static: %w", err)
	}

	log.Info("Done. %d posts built, output: %s", len(posts), outputDir)
	return nil
}

func processFile(mdPath, outputDir string, cssBytes []byte, cssQuery string, embedStyles, tocTop bool, siteCfg SiteConfig, log *logutil.Logger) (post, error) {
	data, err := os.ReadFile(mdPath)
	if err != nil {
		return post{}, fmt.Errorf("reading %s: %w", mdPath, err)
	}

	meta, bodyMD := extractMeta(string(data), filepath.Base(mdPath))

	dir := filepath.Join(outputDir, meta.Date.Format("2006/01/02"), meta.Slug)
	outPath := filepath.Join(dir, "index.html")

	// Link the root style.css with a path relative to this post's directory so
	// it resolves correctly no matter where the blog is mounted (e.g. served
	// from a subdirectory like /blog/). The post lives at
	// outputDir/YYYY/MM/DD/<slug>/, so this yields ../../../../style.css.
	cssHref := "style.css"
	if !embedStyles {
		rel, err := filepath.Rel(dir, outputDir)
		if err != nil {
			return post{}, fmt.Errorf("computing css path for %s: %w", mdPath, err)
		}
		cssHref = filepath.ToSlash(filepath.Join(rel, "style.css"))
	}
	cssHref += cssQuery

	// Skip regeneration when the output HTML is already newer than the source
	// Markdown file and links the current stylesheet version. A changed
	// stylesheet changes the link, so every post is rewritten to point at it.
	mdStat, err := os.Stat(mdPath)
	if err != nil {
		return post{}, fmt.Errorf("stat %s: %w", mdPath, err)
	}
	if outStat, err := os.Stat(outPath); err == nil && !outStat.ModTime().Before(mdStat.ModTime()) && linksCSS(outPath, cssHref) {
		log.Info("Skipped (up to date): %s", outPath)
		return post{Meta: meta, Body: renderMarkdown(bodyMD)}, nil
	}

	rawHTML := renderMarkdown(bodyMD)
	injected, tocEntries := injectHeadingIDs(string(rawHTML))
	toc := buildTOC(tocEntries)
	bodyHTML := template.HTML(injected)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return post{}, fmt.Errorf("mkdir %s: %w", dir, err)
	}

	if err := copyImages(filepath.Dir(mdPath), dir, string(bodyHTML)); err != nil {
		return post{}, fmt.Errorf("copying images: %w", err)
	}
	bodyHTML = template.HTML(fixImagePaths(string(bodyHTML)))

	if embedStyles {
		if err := os.WriteFile(filepath.Join(dir, "style.css"), cssBytes, 0644); err != nil {
			return post{}, fmt.Errorf("writing style.css in post dir: %w", err)
		}
	}

	full, err := wrapPost(meta, bodyHTML, toc, cssHref, tocTop, siteCfg)
	if err != nil {
		return post{}, fmt.Errorf("wrapping post %s: %w", mdPath, err)
	}

	if err := os.WriteFile(outPath, []byte(full), 0644); err != nil {
		return post{}, fmt.Errorf("writing %s: %w", outPath, err)
	}

	log.Info("Built: %s", outPath)
	return post{Meta: meta, Body: bodyHTML}, nil
}

// extractMeta parses the markdown source to extract metadata and the body content.
func extractMeta(src, filename string) (postMeta, string) {
	m := postMeta{
		Date: time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	haveDate := false
	lines := strings.Split(src, "\n")
	metaLines := map[int]bool{}
	titleLineIndex := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "# ") {
			if m.Title == "" {
				m.Title = strings.TrimPrefix(trimmed, "# ")
				titleLineIndex = i
			}
			continue
		}

		if !strings.HasPrefix(trimmed, "- ") {
			continue
		}

		if matches := reCreated.FindStringSubmatch(trimmed); matches != nil {
			if dateMatches := reDate.FindStringSubmatch(matches[1]); dateMatches != nil {
				if t, err := time.Parse("2006/01/02", dateMatches[1]); err == nil {
					m.Date = t
					haveDate = true
				}
			}
			metaLines[i] = true
			continue
		}

		if matches := reTags.FindStringSubmatch(trimmed); matches != nil {
			for _, t := range strings.Split(matches[1], ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					m.Tags = append(m.Tags, t)
				}
			}
			metaLines[i] = true
			continue
		}

		if matches := reDescription.FindStringSubmatch(trimmed); matches != nil {
			m.Description = strings.TrimSpace(matches[1])
			metaLines[i] = true
			continue
		}
	}

	var bodyLines []string
	for i, line := range lines {
		if metaLines[i] {
			continue
		}
		if i == titleLineIndex {
			continue
		}
		bodyLines = append(bodyLines, line)
	}
	body := strings.TrimSpace(strings.Join(bodyLines, "\n"))

	slug := strings.TrimSuffix(filename, filepath.Ext(filename))

	// A "yyyy.mm.dd" filename prefix supplies the date when the post has no
	// Created line, and is dropped from the slug since the path already has it.
	if dm := reFileDate.FindStringSubmatch(slug); dm != nil {
		if t, err := time.Parse("2006.01.02", dm[1]); err == nil {
			if !haveDate {
				m.Date = t
			}
			if rest := slug[len(dm[0]):]; rest != "" {
				slug = rest
			}
		}
	}
	slug = strings.ToLower(slug)
	slug = reSlugClean.ReplaceAllString(slug, "")
	slug = strings.Trim(slug, "-")
	m.Slug = slug

	return m, body
}

type tocEntry struct {
	Level int
	ID    string
	Text  string
}

// injectHeadingIDs adds id attributes to h2–h4 elements and returns the
// modified HTML along with entries for building a table of contents.
func injectHeadingIDs(htmlStr string) (string, []tocEntry) {
	var entries []tocEntry
	idCounts := map[string]int{}

	result := reHeadingTag.ReplaceAllStringFunc(htmlStr, func(match string) string {
		parts := reHeadingTag.FindStringSubmatch(match)
		if len(parts) < 3 {
			return match
		}
		level := int(parts[1][0] - '0')
		inner := parts[2]

		plain := reStripTags.ReplaceAllString(inner, "")
		id := strings.ToLower(plain)
		id = strings.ReplaceAll(id, " ", "-")
		id = reSlugClean.ReplaceAllString(id, "")
		id = strings.Trim(id, "-")
		if id == "" {
			id = fmt.Sprintf("heading-%d", level)
		}

		base := id
		if idCounts[base] > 0 {
			id = fmt.Sprintf("%s-%d", base, idCounts[base])
		}
		idCounts[base]++

		entries = append(entries, tocEntry{Level: level, ID: id, Text: template.HTMLEscapeString(plain)})
		return fmt.Sprintf(`<h%d id="%s">%s</h%d>`, level, id, inner, level)
	})

	return result, entries
}

func buildTOC(entries []tocEntry) template.HTML {
	if len(entries) < 2 {
		return ""
	}
	var buf strings.Builder
	buf.WriteString(`<nav class="toc"><ol>`)
	for _, e := range entries {
		buf.WriteString(fmt.Sprintf(`<li class="toc-h%d"><a href="#%s">%s</a></li>`, e.Level, e.ID, e.Text))
	}
	buf.WriteString(`</ol></nav>`)
	return template.HTML(buf.String())
}

func wrapPost(m postMeta, bodyHTML template.HTML, toc template.HTML, cssHref string, tocTop bool, siteCfg SiteConfig) (string, error) {
	tmpl, err := template.New("post").Parse(postTemplate)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	data := struct {
		Meta      postMeta
		Body      template.HTML
		TOC       template.HTML
		TOCTop    bool
		CSSHref   string
		Site      SiteConfig
		HasTweets bool
	}{
		Meta:      m,
		Body:      bodyHTML,
		TOC:       toc,
		TOCTop:    tocTop,
		CSSHref:   cssHref,
		Site:      siteCfg,
		HasTweets: hasTweets(string(bodyHTML)),
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// writeBlogListing renders blog_entries.html into outputDir. root is the
// relative path from outputDir back to the site root (e.g. "../../" for tag
// pages) and is prefixed to every post link.
func writeBlogListing(outputDir, root string, posts []post, siteCfg SiteConfig, cssQuery string) error {
	tmpl, err := template.New("listing").Parse(listingTemplate)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	data := struct {
		Posts    []post
		Site     SiteConfig
		CSSQuery string
		Root     string
	}{
		Posts:    posts,
		Site:     siteCfg,
		CSSQuery: cssQuery,
		Root:     root,
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		return err
	}

	outPath := filepath.Join(outputDir, "blog_entries.html")
	// Only write if content actually changed to avoid bumping the mtime.
	if existing, err := os.ReadFile(outPath); err == nil && bytes.Equal(existing, buf.Bytes()) {
		return nil
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", outputDir, err)
	}
	return os.WriteFile(outPath, buf.Bytes(), 0644)
}

// cssVersionQuery returns a "?v=<hash>" suffix for style.css links. The hash
// follows the stylesheet's content, so browsers and CDNs that cached an older
// copy fetch the new one as soon as it changes.
func cssVersionQuery(cssBytes []byte) string {
	sum := sha256.Sum256(cssBytes)
	return "?v=" + hex.EncodeToString(sum[:4])
}

// linksCSS reports whether the HTML file at path links the stylesheet href.
func linksCSS(path, href string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return bytes.Contains(data, []byte(`href="`+template.HTMLEscapeString(href)+`"`))
}

func buildCSS() []byte {
	var buf bytes.Buffer
	buf.WriteString(defaultCSS)

	// Add syntax highlighting CSS
	formatter := html.New(html.WithClasses(true))

	// Light theme (GitHub)
	buf.WriteString("\n/* Syntax Highlighting - Light */\n")
	if style := styles.Get("github"); style != nil {
		formatter.WriteCSS(&buf, style)
	}

	// Dark theme (Dracula)
	buf.WriteString("\n@media (prefers-color-scheme: dark) {\n")
	if style := styles.Get("dracula"); style != nil {
		formatter.WriteCSS(&buf, style)
	}
	buf.WriteString("}\n")

	return buf.Bytes()
}

func copyStatic(outputDir string, cssBytes []byte) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", outputDir, err)
	}
	cssPath := filepath.Join(outputDir, "style.css")
	// Only write if content actually changed to avoid bumping the mtime.
	if existing, err := os.ReadFile(cssPath); err == nil && bytes.Equal(existing, cssBytes) {
		return nil
	}
	return os.WriteFile(cssPath, cssBytes, 0644)
}

func writeTagIndexes(outputDir string, posts []post, siteCfg SiteConfig, cssBytes []byte, cssQuery string) error {
	tagPosts := map[string][]post{}
	for _, p := range posts {
		for _, tag := range p.Meta.Tags {
			tagPosts[tag] = append(tagPosts[tag], p)
		}
	}

	for tag, tPosts := range tagPosts {
		tagDir := filepath.Join(outputDir, "tags", reSlugClean.ReplaceAllString(strings.ToLower(tag), ""))

		cfg := siteCfg
		cfg.Title = fmt.Sprintf("Tag: %s - %s", tag, siteCfg.Title)
		if err := writeBlogListing(tagDir, "../../", tPosts, cfg, cssQuery); err != nil {
			return err
		}
		// Copy style.css so the relative link works
		if err := copyStatic(tagDir, cssBytes); err != nil {
			return err
		}
	}
	return nil
}

func copyImages(mdDir, outDir, html string) error {
	matches := reImgSrc.FindAllStringSubmatch(html, -1)
	for _, m := range matches {
		src := m[1]
		if strings.HasPrefix(src, "http") || strings.HasPrefix(src, "/") {
			continue
		}
		candidates := []string{
			filepath.Join(mdDir, src),
			filepath.Join(mdDir, "images", filepath.Base(src)),
			filepath.Join(mdDir, strings.TrimPrefix(src, "./")),
			filepath.Join(mdDir, strings.TrimPrefix(src, "../")),
		}
		var data []byte
		var ok bool
		for _, c := range candidates {
			if d, err := os.ReadFile(c); err == nil {
				data = d
				ok = true
				break
			}
		}
		if !ok {
			continue
		}
		dstPath := filepath.Join(outDir, filepath.Base(src))
		// Skip overwriting if the destination already has the same size.
		if dstStat, err := os.Stat(dstPath); err == nil && dstStat.Size() == int64(len(data)) {
			continue
		}
		if err := os.WriteFile(dstPath, data, 0644); err != nil {
			return fmt.Errorf("copying image %s: %w", dstPath, err)
		}
	}
	return nil
}

func fixImagePaths(html string) string {
	return reFixImgPath.ReplaceAllString(html, "$1$2")
}

func renderMarkdown(md string) template.HTML {
	var buf bytes.Buffer
	mdRenderer := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Typographer,
			highlighting.NewHighlighting(
				highlighting.WithFormatOptions(
					html.WithClasses(true),
				),
			),
		),
	)
	if err := mdRenderer.Convert([]byte(md), &buf); err != nil {
		return template.HTML(fmt.Sprintf("<p>error rendering markdown: %v</p>", err))
	}
	return template.HTML(embedTweets(transformCallouts(buf.String())))
}

var calloutRe = regexp.MustCompile(`(?si)<blockquote>\s*<p>\[!(info|warning|tip|success|note|danger|error|important|caution)\]\s*(.*?)</p>(.*?)</blockquote>`)

func transformCallouts(html string) string {
	return calloutRe.ReplaceAllStringFunc(html, func(match string) string {
		parts := calloutRe.FindStringSubmatch(match)
		if len(parts) < 4 {
			return match
		}
		typ := strings.ToLower(parts[1])
		return fmt.Sprintf(`<div class="callout callout-%s"><p>%s</p>%s</div>`, typ, parts[2], parts[3])
	})
}

const rssTemplate = `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">
  <channel>
    <title>{{.Site.Title}}</title>
    <link>{{.Site.BaseURL}}</link>
    <description>{{.Site.Description}}</description>
    <atom:link href="{{.Site.BaseURL}}/feed.xml" rel="self" type="application/rss+xml" />
    {{range .Posts}}
    <item>
      <title>{{.Meta.Title}}</title>
      <link>{{$.Site.BaseURL}}/{{.Meta.Date.Format "2006/01/02"}}/{{.Meta.Slug}}/index.html</link>
      <guid>{{$.Site.BaseURL}}/{{.Meta.Date.Format "2006/01/02"}}/{{.Meta.Slug}}/index.html</guid>
      <pubDate>{{.Meta.Date.Format "Mon, 02 Jan 2006 15:04:05 -0700"}}</pubDate>
      <description>{{.Meta.Description}}</description>
    </item>
    {{end}}
  </channel>
</rss>`

func writeRSS(outputDir string, posts []post, siteCfg SiteConfig) error {
	tmpl, err := template.New("rss").Parse(rssTemplate)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	data := struct {
		Posts []post
		Site  SiteConfig
	}{
		Posts: posts,
		Site:  siteCfg,
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		return err
	}

	outPath := filepath.Join(outputDir, "feed.xml")
	return os.WriteFile(outPath, buf.Bytes(), 0644)
}

const sitemapTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>{{.Site.BaseURL}}/</loc>
  </url>
  {{range .Posts}}
  <url>
    <loc>{{$.Site.BaseURL}}/{{.Meta.Date.Format "2006/01/02"}}/{{.Meta.Slug}}/index.html</loc>
  </url>
  {{end}}
</urlset>`

func writeSitemap(outputDir string, posts []post, siteCfg SiteConfig) error {
	tmpl, err := template.New("sitemap").Parse(sitemapTemplate)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	data := struct {
		Posts []post
		Site  SiteConfig
	}{
		Posts: posts,
		Site:  siteCfg,
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		return err
	}

	outPath := filepath.Join(outputDir, "sitemap.xml")
	return os.WriteFile(outPath, buf.Bytes(), 0644)
}
