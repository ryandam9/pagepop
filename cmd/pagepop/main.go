package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"pagepop/internal/builder"
	"pagepop/internal/logutil"
)

const version = "0.1.0"

const helpText = `Pagepop — a lightweight static site generator

Converts markdown files listed in a YAML config into a self-contained
static blog ready to serve from any static host.

Usage:
  pagepop [flags]

Output layout:
  <output>/
    style.css
    blog_entries.html
    YYYY/
      MM/
        DD/
          <slug>/
            index.html
            <assets>

Flags:
`

func main() {
	var cfgPath string
	flag.StringVar(&cfgPath, "config", "md_files.yml", "Path to the YAML config file listing markdown files")
	flag.StringVar(&cfgPath, "c", "md_files.yml", "Short form of --config")

	var outPath string
	flag.StringVar(&outPath, "output", "blog", "Output directory for the built site")
	flag.StringVar(&outPath, "o", "blog", "Short form of --output")

	var verbose bool
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose (debug) logging")
	flag.BoolVar(&verbose, "v", false, "Short form of --verbose")

	embedStyles := flag.Bool("embed-styles", false, "Copy style.css into each post directory for standalone pages")
	clean := flag.Bool("clean", false, "Wipe the output directory before building")
	tocTop := flag.Bool("toc-top", false, "Render the table of contents at the top of the post instead of in a left sidebar on large screens")
	showVersion := flag.Bool("version", false, "Print version and exit")

	flag.Usage = func() {
		fmt.Fprint(os.Stderr, helpText)
		flag.PrintDefaults()
		fmt.Fprint(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  pagepop                         Use defaults (md_files.yml → ./blog)\n")
		fmt.Fprintf(os.Stderr, "  pagepop -c my-posts.yml         Custom config file\n")
		fmt.Fprintf(os.Stderr, "  pagepop -c my.yml -o ./dist     Custom config and output\n")
		fmt.Fprintf(os.Stderr, "  pagepop --embed-styles          Copy CSS into each post dir\n")
		fmt.Fprintf(os.Stderr, "  pagepop --clean                 Wipe output directory before build\n")
		fmt.Fprintf(os.Stderr, "  pagepop --toc-top               Place the TOC at the top instead of the side\n")
		fmt.Fprintf(os.Stderr, "  pagepop --version               Show version\n")
		fmt.Fprintf(os.Stderr, "  pagepop --verbose               Enable verbose logging\n")
	}

	flag.Parse()

	log := logutil.New(logutil.LevelInfo)
	if verbose {
		log.SetLevel(logutil.LevelDebug)
	}

	if flag.NFlag() == 0 {
		flag.Usage()
		os.Exit(0)
	}

	if *showVersion {
		fmt.Printf("pagepop v%s\n", version)
		os.Exit(0)
	}

	if cfgPath == "" {
		log.Error("--config path is required")
		os.Exit(1)
	}

	absOutput, err := filepath.Abs(outPath)
	if err != nil {
		log.Error("could not resolve output path: %v", err)
		os.Exit(1)
	}

	if err := builder.Site(absOutput, cfgPath, *embedStyles, *clean, *tocTop, log); err != nil {
		var pathErr *os.PathError
		if errors.As(err, &pathErr) {
			log.Error("%v", pathErr)
			os.Exit(1)
		}
		log.Error("building site: %v", err)
		os.Exit(1)
	}
}
