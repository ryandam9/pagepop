# Pagepop

A lightweight static site generator written in Go. Point it at a list of Markdown files and it produces a self-contained blog directory ready to serve from any static host.

## Features

- **Markdown to HTML** — powered by [`goldmark`](https://github.com/yuin/goldmark) with GFM and typographer extensions
- **Syntax highlighting** — automatic light/dark code-block themes via [`chroma`](https://github.com/alecthomas/chroma)
- **Built-in styles** — clean, responsive default theme; no external CSS frameworks
- **Image lightbox** — click any image in a post to expand it to a full-screen view
- **Smooth navigation** — table-of-contents links glide to their section instead of jumping
- **HTML pages** — publish ready-made HTML pages (single files or whole folders) alongside the posts
- **Embedded X posts** — a link to a post on X/Twitter on its own line becomes an embedded post
- **Zero config** — only a YAML list of Markdown files is required

## Output layout

Each post is placed under a date-based path derived from its `Created` front matter field:

```
blog/
├── style.css                        # shared stylesheet
├── blog_entries.html                # post listing (newest first)
└── YYYY/
    └── MM/
        └── DD/
            └── <slug>/
                ├── index.html       # rendered post
                └── <images>         # images referenced in the post
```

The slug is the Markdown filename with the extension removed, lowercased, and non-alphanumeric characters stripped. If the filename starts with a `yyyy.mm.dd` date (e.g. `2026.10.09-my-post.md`), that prefix is dropped from the slug, and it supplies the post date when there is no `Created` line. Posts with no date from either source fall back to `1900/01/01`.

## Getting started

**Prerequisites:** Go 1.26 or later.

```bash
git clone https://github.com/yourusername/pagepop.git
cd pagepop
make build
```

The binary is written to `bin/pagepop`.

## Usage

Create a YAML file that lists the Markdown files you want to publish:

```yaml
# md_files.yml
markdown_files:
  - file: /path/to/hello-world.md
  - file: /path/to/second-post.md
```

Paths can be absolute or relative to the working directory.

Then run:

```bash
./bin/pagepop --config md_files.yml --output ./blog
```

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `md_files.yml` | Path to the YAML file listing Markdown files |
| `--output` | `blog` | Output directory (created if it doesn't exist) |
| `--embed-styles` | `false` | Copy `style.css` into each post directory for standalone pages |
| `--toc-top` | `false` | Render the table of contents at the top of the post instead of in a left sidebar on large screens |

By default the table of contents sits in a sticky left sidebar on wide
screens (≥1200px) and collapses above the content on narrower ones. Pass
`--toc-top` to always place it at the top, right after the title.

With `--embed-styles`, each post becomes fully self-contained:

```
blog/
├── style.css
├── blog_entries.html
└── YYYY/
    └── MM/
        └── DD/
            └── <slug>/
                ├── style.css
                ├── index.html
                └── <images>
```

You can also use `make run`, with optional overrides:

```bash
make run CONFIG=my_posts.yml OUTPUT=./dist
```

## Markdown front matter

Metadata is read from structured list items at the top of each file. The `# Title` heading and all recognised metadata lines are stripped from the rendered body.

```markdown
# Post Title

- Created - 2024/06/01
- Tags - go, web, tools
- Description - A short summary shown in the listing.

Body content starts here...
```

| Field | Format | Required | Description |
|-------|--------|----------|-------------|
| `Created` | `YYYY/MM/DD` | Recommended | Publication date — sets the output path and listing sort order |
| `Tags` | comma-separated | No | Labels shown on the post and in the listing |
| `Description` | plain text | No | Subtitle shown below the title and in the listing |

> **Note:** If `Created` is missing the post is placed under `1900/01/01/` and sorts to the bottom of the listing.

## HTML pages

Ready-made HTML pages can be published next to the posts and appear in the
listing like any other entry. List them under `html_pages`:

```yaml
html_pages:
  # A single self-contained file.
  - file: /path/to/standalone.html
    created: 2026/10/01

  # A folder holding the page and everything it loads. The whole folder is
  # copied (dot-files such as .git are skipped), so relative links keep working.
  - dir: /path/to/my-demo
    index: demo.html               # entry page; default index.html
    created: 2026/10/02
    title: My demo                 # default: the page's <title>
    description: What it shows     # default: its <meta name="description">
    tags: [javascript, demo]
    slug: my-demo                  # default: the file or folder name
```

Each page lands at `YYYY/MM/DD/<slug>/index.html`, just like a post. Pages are
published as-is: pagepop does not wrap them in the blog's layout or styles. An
entry page not named `index.html` is also published under that name, so the
folder must not already contain a different `index.html`.

## Embedded X (Twitter) posts

Put a link to a post on a line of its own and it renders as an embedded post:

```markdown
Here is what they said:

https://x.com/jack/status/20
```

Bare URLs, `<https://...>` and `[text](https://...)` all work, for both
`x.com` and `twitter.com`. A link inside a sentence stays an ordinary link.
The embed script (`platform.twitter.com/widgets.js`) is loaded only on posts
that contain one, with X's "do not track" option set, and it follows the
reader's light/dark preference. Without JavaScript the reader sees a plain
link to the post.

## Behaviour notes

- **Up-to-date check** — a post is skipped if its `index.html` is already newer than the source `.md` file, avoiding unnecessary rebuilds.
- **Image copying** — images referenced in a post are copied into the post's output directory. Supported lookup paths: `images/<file>`, `./images/<file>`, `../<file>`.
- **Image lightbox** — every image in a post body is clickable. Clicking expands it to a centred, full-screen view over a dimmed backdrop; it closes on `Escape`, on clicking the backdrop, or via the ✕ button. The full-resolution source is shown, so large SVG/PNG/JPEG images fill the screen instead of being constrained to the post column.
- **Smooth scrolling** — clicking a table-of-contents link smoothly scrolls to the target heading. Readers who enable the OS "reduce motion" setting get an instant jump instead.
- **Missing files** — entries in the config that cannot be read are logged as warnings and skipped; the rest of the build continues.

## Development

```bash
make build   # compile the binary
make test    # run unit tests
make lint    # go vet
make fmt     # gofmt
make tidy    # go mod tidy
make all     # fmt + tidy + lint + test + build
```

## License

MIT — see [LICENSE](LICENSE).
