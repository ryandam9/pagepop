package builder

import (
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"pagepop/internal/logutil"
)

// htmlEntry is a ready-made HTML page published as-is alongside the Markdown
// posts. It is either a single self-contained file or a directory holding the
// page plus whatever it loads (scripts, styles, images, data).
//
//	html_pages:
//	  - file: /path/to/standalone.html
//	    created: 2026/10/01
//	  - dir: /path/to/my-demo        # copied whole; dot-files are skipped
//	    index: demo.html             # entry page, default index.html
//	    title: My demo               # default: the page's <title>
//	    description: What it shows   # default: its meta description
//	    tags: [javascript, demo]
//	    slug: my-demo                # default: file or directory name
type htmlEntry struct {
	File        string   `yaml:"file"`
	Dir         string   `yaml:"dir"`
	Index       string   `yaml:"index"`
	Title       string   `yaml:"title"`
	Created     string   `yaml:"created"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags"`
	Slug        string   `yaml:"slug"`
}

var (
	reHTMLTitle = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	reHTMLDesc  = regexp.MustCompile(`(?is)<meta\s+[^>]*name=["']description["'][^>]*>`)
	reContent   = regexp.MustCompile(`(?is)content=["']([^"']*)["']`)
)

// processHTMLPage copies an HTML page (or page directory) to
// outputDir/YYYY/MM/DD/<slug>/ so that the listing can link to its
// index.html exactly as it does for a Markdown post.
func processHTMLPage(e htmlEntry, outputDir string, log *logutil.Logger) (post, error) {
	if (e.File == "") == (e.Dir == "") {
		return post{}, fmt.Errorf("html_pages entry needs exactly one of file or dir")
	}

	srcRoot, entry := filepath.Dir(e.File), filepath.Base(e.File)
	name := strings.TrimSuffix(entry, filepath.Ext(entry))
	if e.Dir != "" {
		srcRoot, entry = filepath.Clean(e.Dir), e.Index
		if entry == "" {
			entry = "index.html"
		}
		name = filepath.Base(srcRoot)
	}
	entryPath := filepath.Join(srcRoot, entry)

	data, err := os.ReadFile(entryPath)
	if err != nil {
		return post{}, fmt.Errorf("reading %s: %w", entryPath, err)
	}

	meta := postMeta{
		Title:       e.Title,
		Description: e.Description,
		Tags:        e.Tags,
		Date:        time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if meta.Title == "" {
		if m := reHTMLTitle.FindSubmatch(data); m != nil {
			meta.Title = strings.TrimSpace(html.UnescapeString(string(m[1])))
		}
	}
	if meta.Title == "" {
		meta.Title = name
	}
	if meta.Description == "" {
		if tag := reHTMLDesc.Find(data); tag != nil {
			if m := reContent.FindSubmatch(tag); m != nil {
				meta.Description = strings.TrimSpace(html.UnescapeString(string(m[1])))
			}
		}
	}
	if d := reDate.FindString(e.Created); d != "" {
		if t, err := time.Parse("2006/01/02", d); err == nil {
			meta.Date = t
		}
	} else {
		log.Warn("%s: no created date (YYYY/MM/DD), placing it under 1900/01/01", entryPath)
	}

	slug := e.Slug
	if slug == "" {
		slug = name
	}
	slug = strings.Trim(reSlugClean.ReplaceAllString(strings.ToLower(slug), ""), "-")
	if slug == "" {
		return post{}, fmt.Errorf("%s: cannot derive a slug, set slug explicitly", entryPath)
	}
	meta.Slug = slug

	dir := filepath.Join(outputDir, meta.Date.Format("2006/01/02"), slug)
	if e.Dir != "" {
		if err := copyTree(srcRoot, dir); err != nil {
			return post{}, err
		}
		// The listing (and anything that discovers posts by index.html) links
		// to index.html, so a differently named entry page is also published
		// under that name. Relative links keep working: it is the same folder.
		if entry != "index.html" {
			if _, err := os.Stat(filepath.Join(srcRoot, "index.html")); err == nil {
				return post{}, fmt.Errorf("%s: has its own index.html, so index: %s cannot be published as index.html", srcRoot, entry)
			}
			if err := copyIfNewer(entryPath, filepath.Join(dir, "index.html")); err != nil {
				return post{}, err
			}
		}
	} else if err := copyIfNewer(entryPath, filepath.Join(dir, "index.html")); err != nil {
		return post{}, err
	}

	log.Info("Published HTML: %s", filepath.Join(dir, "index.html"))
	return post{Meta: meta}, nil
}

// copyTree copies src into dst, skipping dot-files and dot-directories (.git,
// .DS_Store, .env, ...) so that a working directory can be published directly.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel != "." && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return copyIfNewer(path, target)
	})
}

// copyIfNewer copies src to dst unless dst is already at least as new, so a
// rebuild leaves unchanged files (and their mtimes) alone.
func copyIfNewer(src, dst string) error {
	srcStat, err := os.Stat(src)
	if err != nil {
		return err
	}
	if dstStat, err := os.Stat(dst); err == nil && !dstStat.ModTime().Before(srcStat.ModTime()) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("copying %s: %w", src, err)
	}
	return out.Close()
}
