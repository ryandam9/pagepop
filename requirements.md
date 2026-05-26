Pagepop — Code Review & Ideas

## Summary

This is a clean, well-structured Go static site generator. The code is idiomatic, the dependency set is minimal and well-chosen, and the feature set is coherent. Below I've organized my findings into **bugs/issues**, **gaps**, and **new ideas**.

---

## 🐛 Bugs & Issues

### 1. Short flags `-c` / `-o` don't actually work

`flag.String` registers each flag name independently — `-c` and `--config` are two separate variables with the same default. The precedence logic in `main.go` only picks up the short form when the long form is still at its default:

```go
if *config == "md_files.yml" && *configShort != "md_files.yml" {
    cfgPath = *configShort
}
```

This means:
- `pagepop -c custom.yml --config other.yml` → silently uses `other.yml`, ignoring `-c`.
- `pagepop -c md_files.yml` → treated as the default, effectively a no-op.
- There's no way to intentionally pass `md_files.yml` via the short flag.

**Fix:** Use a single `flag.String` per logical flag and register an alias via `flag.StringVar`, or switch to a CLI library that handles aliases natively (e.g. `pflag`, `cobra`, `kong`).

### 2. `--embed-styles` breaks the listing page CSS path

When `--embed-styles` is enabled, each post gets a local `style.css`. But the listing template (`listing.html`) always links to `style.css` (relative), which works. However the post template's CSS href becomes `"style.css"` (relative to the post), while the listing is at the root — this is fine. The *real* problem is that **`/style.css` is still written to the root** even in embed mode. If the intent of `--embed-styles` is full portability without a root, the listing page itself isn't self-contained (it still expects the root-level CSS but doesn't embed it inline).

### 3. CSS `rgba()` used incorrectly with CSS variables

In `style.css`:
```css
.nav { background: rgba(var(--bg-card), 0.8); }
.post-body tr:nth-child(even) { background: rgba(var(--bg-code), 0.3); }
```
`var(--bg-card)` resolves to `#ffffff` or `#18181b` — a hex string, not separate R/G/B channels. `rgba(#ffffff, 0.8)` is invalid in most browsers. This makes the nav backdrop-filter and alternating table rows not render as intended.

**Fix:** Define separate `--bg-card-rgb: 255, 255, 255` variables and use `rgba(var(--bg-card-rgb), 0.8)`, or use the `color-mix()` function.

### 4. Skipped posts still appear in the listing with empty `Body`

When a post is skipped due to the up-to-date check, `processFile` returns `post{Meta: meta}` with `Body` set to the zero-value (`""`). These posts are still appended to the `posts` slice and passed to `writeBlogListing`. The listing only uses `Meta`, so it renders fine today, but this is fragile — any future template logic that checks `Body` would behave unexpectedly.

### 5. `extractMeta` skips *all* `# Title` lines, not just the first

The title-extraction loop `continue`s on every line starting with `# `, which means if the body contains a top-level heading, it's silently removed from the output. This is likely unintentional for posts that use `# ` for section headers in the body below the front-matter block.

### 6. Makefile `deploy` target referenced in `CLAUDE.md` but not defined

`CLAUDE.md` says `make deploy` does "Full site assembly", but the Makefile has no `deploy` target.

---

## ⚠️ Gaps

| Area | Gap |
|---|---|
| **Test coverage** | Only `extractMeta` and `fixImagePaths` are tested. No tests for `renderMarkdown`, `wrapPost`, `writeBlogListing`, `copyImages`, `buildCSS`, or the end-to-end `Site` flow. |
| **Error handling in `copyImages`** | `os.WriteFile` errors are silently discarded (`_ = os.WriteFile(...)`). A permissions or disk-full failure would be invisible. |
| **No RSS/Atom feed** | Common expectation for a blog generator. |
| **No sitemap.xml** | Helps with SEO / crawlability. |
| **No 404 page** | Most static hosts can serve a custom `404.html`. |
| **Hardcoded site title** | The listing page title is hardcoded to `"Blog"`. There's no way to customize the site name, author, or base URL. |
| **No `<meta og:*>` or Twitter card tags** | Posts have `<meta description>` but no Open Graph or social sharing metadata. |
| **`assets/` directory missing** | `CLAUDE.md` references `assets/` for site-specific config and static assets, but the directory doesn't exist in the repo. |
| **No `--dry-run` or `--verbose` flag** | Would help with debugging without writing files. |
| **No `--watch` / live-reload** | Common DX feature for static site generators. |
| **No CI config** | No GitHub Actions, Makefile `deploy` target, or similar automation. |
| **README says "Go 1.21 or later" but `go.mod` says `go 1.26.1`** | The minimum version claim is outdated/wrong. |
| **Google Fonts fetched at runtime** | `Overpass Mono` is loaded from Google Fonts CDN — breaks offline use and adds a third-party dependency. Consider self-hosting the font or making it optional. |
| **`GEMINI.md`** | Exists but not referenced — presumably a parallel agent instructions file. Fine, but worth noting. |

---

## 💡 New Ideas

### Near-term (low effort, high value)

1. **Site-level config in YAML** — Extend the config file (or add a top-level `site:` key) to support `title`, `author`, `base_url`, `description`, `language`. Thread these into templates for proper `<title>`, OG tags, and RSS links.

2. **RSS feed generation** — After building posts, emit an `feed.xml` (Atom or RSS 2.0). You already have all the metadata; it's ~50 lines of template.

3. **Sitemap generation** — Similarly, emit `sitemap.xml` with `<url>` entries for each post + the index.

4. **End-to-end test** — Write a `TestSite` that creates a temp dir with a YAML config + a couple of `.md` files, calls `Site()`, and asserts the output tree structure and key HTML content. This would catch regressions in the image-copy, CSS-embed, and listing flows.

5. **Tag index pages** — Generate `/tags/<tag>/index.html` pages listing posts for each tag. The data structure already supports this.

6. **`--clean` flag** — Optionally wipe the output directory before building, to remove stale posts that were deleted from the config.

### Medium-term (moderate effort)

7. **Draft support** — Add a `Draft: true` front-matter field. Skip drafts by default; add `--include-drafts` flag for preview builds.

8. **Custom templates** — Allow `--templates <dir>` to override the embedded `post.html` / `listing.html`. Fall back to the embedded defaults. This makes the generator reusable across different designs.

9. **Pagination** — For blogs with many posts, paginate the listing (e.g. 10 posts per page) and generate `page/2/index.html`, etc.

10. **`--serve` mode** — Embed `net/http` to serve the output directory on `localhost` with auto-rebuild on file change. Go's stdlib makes this straightforward.

11. **Prev/Next navigation** — Since posts are already sorted by date, inject "← Previous" / "Next →" links into each post template.

12. **Reading time estimate** — Count words in the body, divide by ~200 WPM, and expose `{{.Meta.ReadingTime}}` to templates.
