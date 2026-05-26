package builder

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

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

type mdEntry struct {
	File string `yaml:"file"`
}

type mdConfig struct {
	Site          SiteConfig `yaml:"site"`
	MarkdownFiles []mdEntry  `yaml:"markdown_files"`
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
	reTags        = regexp.MustCompile(`(?i)^-\s*tags\s*-\s*(.+)`)
	reDescription = regexp.MustCompile(`(?i)^-\s*description\s*-\s*(.+)`)
	reCreated     = regexp.MustCompile(`(?i)^-\s*created\s*-\s*(.+)`)
	reImgSrc      = regexp.MustCompile(`<img\s+[^>]*src="([^"]+)"`)
	reFixImgPath  = regexp.MustCompile(`(<img\s+[^>]*src=")(?:\.\./|\./)?(?:images/)?([^"]+")`)
	reSlugClean   = regexp.MustCompile(`[^a-z0-9-]`)
)

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
func Site(outputDir, configPath string, embedStyles, clean bool) error {
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

	var posts []post

	for _, entry := range cfg.MarkdownFiles {
		p, err := processFile(entry.File, outputDir, cssBytes, embedStyles, cfg.Site)
		if err != nil {
			log.Printf("WARN: processing file %s: %v", entry.File, err)
			continue
		}
		posts = append(posts, p)
	}

	sort.Slice(posts, func(i, j int) bool {
		return posts[i].Meta.Date.After(posts[j].Meta.Date)
	})

	if err := writeBlogListing(outputDir, posts, cfg.Site); err != nil {
		return fmt.Errorf("writing blog listing: %w", err)
	}

	if err := writeTagIndexes(outputDir, posts, cfg.Site, cssBytes); err != nil {
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

	log.Printf("Done. %d posts built, output: %s", len(posts), outputDir)
	return nil
}

func processFile(mdPath, outputDir string, cssBytes []byte, embedStyles bool, siteCfg SiteConfig) (post, error) {
	data, err := os.ReadFile(mdPath)
	if err != nil {
		return post{}, fmt.Errorf("reading %s: %w", mdPath, err)
	}

	meta, bodyMD := extractMeta(string(data), filepath.Base(mdPath))

	dir := filepath.Join(outputDir, meta.Date.Format("2006/01/02"), meta.Slug)
	outPath := filepath.Join(dir, "index.html")

	// Skip regeneration when the output HTML is already newer than the source
	// Markdown file.
	mdStat, err := os.Stat(mdPath)
	if err != nil {
		return post{}, fmt.Errorf("stat %s: %w", mdPath, err)
	}
	if outStat, err := os.Stat(outPath); err == nil && !outStat.ModTime().Before(mdStat.ModTime()) {
		log.Printf("Skipped (up to date): %s", outPath)
		return post{Meta: meta, Body: renderMarkdown(bodyMD)}, nil
	}

	bodyHTML := renderMarkdown(bodyMD)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return post{}, fmt.Errorf("mkdir %s: %w", dir, err)
	}

	if err := copyImages(filepath.Dir(mdPath), dir, string(bodyHTML)); err != nil {
		return post{}, fmt.Errorf("copying images: %w", err)
	}
	bodyHTML = template.HTML(fixImagePaths(string(bodyHTML)))

	cssHref := "/style.css"
	if embedStyles {
		cssHref = "style.css"
		if err := os.WriteFile(filepath.Join(dir, "style.css"), cssBytes, 0644); err != nil {
			return post{}, fmt.Errorf("writing style.css in post dir: %w", err)
		}
	}

	full, err := wrapPost(meta, bodyHTML, cssHref, siteCfg)
	if err != nil {
		return post{}, fmt.Errorf("wrapping post %s: %w", mdPath, err)
	}

	if err := os.WriteFile(outPath, []byte(full), 0644); err != nil {
		return post{}, fmt.Errorf("writing %s: %w", outPath, err)
	}

	log.Printf("Built: %s", outPath)
	return post{Meta: meta, Body: bodyHTML}, nil
}

// extractMeta parses the markdown source to extract metadata and the body content.
func extractMeta(src, filename string) (postMeta, string) {
	m := postMeta{
		Date: time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC),
	}

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
	slug = strings.ToLower(slug)
	slug = reSlugClean.ReplaceAllString(slug, "")
	slug = strings.Trim(slug, "-")
	m.Slug = slug

	return m, body
}

func wrapPost(m postMeta, bodyHTML template.HTML, cssHref string, siteCfg SiteConfig) (string, error) {
	tmpl, err := template.New("post").Parse(postTemplate)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	data := struct {
		Meta    postMeta
		Body    template.HTML
		CSSHref string
		Site    SiteConfig
	}{
		Meta:    m,
		Body:    bodyHTML,
		CSSHref: cssHref,
		Site:    siteCfg,
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func writeBlogListing(outputDir string, posts []post, siteCfg SiteConfig) error {
	tmpl, err := template.New("listing").Parse(listingTemplate)
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

func writeTagIndexes(outputDir string, posts []post, siteCfg SiteConfig, cssBytes []byte) error {
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
		if err := writeBlogListing(tagDir, tPosts, cfg); err != nil {
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
	md = strings.ReplaceAll(md, "[!info]", "")
	md = strings.ReplaceAll(md, "[!warning]", "")

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
	return template.HTML(buf.String())
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
