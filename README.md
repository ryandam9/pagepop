# Pagepop

A lightweight static site generator written in Go. Point it at a list of Markdown files and it produces a self-contained blog directory ready to serve from any static host.

## Features

- **Markdown to HTML** — powered by [`goldmark`](https://github.com/yuin/goldmark) with GFM and typographer extensions
- **Syntax highlighting** — automatic light/dark code-block themes via [`chroma`](https://github.com/alecthomas/chroma)
- **Built-in styles** — clean, responsive default theme; no external CSS frameworks
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

The slug is the Markdown filename with the extension removed, lowercased, and non-alphanumeric characters stripped. Posts missing a `Created` date fall back to `1900/01/01`.

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

## Behaviour notes

- **Up-to-date check** — a post is skipped if its `index.html` is already newer than the source `.md` file, avoiding unnecessary rebuilds.
- **Image copying** — images referenced in a post are copied into the post's output directory. Supported lookup paths: `images/<file>`, `./images/<file>`, `../<file>`.
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
