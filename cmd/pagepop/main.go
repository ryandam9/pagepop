package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"pagepop/internal/builder"
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
	log.SetFlags(0)

	var cfgPath string
	flag.StringVar(&cfgPath, "config", "md_files.yml", "Path to the YAML config file listing markdown files")
	flag.StringVar(&cfgPath, "c", "md_files.yml", "Short form of --config")

	var outPath string
	flag.StringVar(&outPath, "output", "blog", "Output directory for the built site")
	flag.StringVar(&outPath, "o", "blog", "Short form of --output")

	embedStyles := flag.Bool("embed-styles", false, "Copy style.css into each post directory for standalone pages")
	clean := flag.Bool("clean", false, "Wipe the output directory before building")
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
		fmt.Fprintf(os.Stderr, "  pagepop --version               Show version\n")
	}

	flag.Parse()

	if flag.NFlag() == 0 {
		flag.Usage()
		os.Exit(0)
	}

	if *showVersion {
		fmt.Printf("pagepop v%s\n", version)
		os.Exit(0)
	}

	if cfgPath == "" {
		log.Fatal("Error: --config path is required")
	}

	absOutput, err := filepath.Abs(outPath)
	if err != nil {
		log.Fatalf("Error: could not resolve output path: %v", err)
	}

	if err := builder.Site(absOutput, cfgPath, *embedStyles, *clean); err != nil {
		var pathErr *os.PathError
		if errors.As(err, &pathErr) {
			log.Fatalf("Error: %v", pathErr)
		}
		log.Fatalf("Error building site: %v", err)
	}
}
