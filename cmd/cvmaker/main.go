// Command cvmaker turns a CV written in Markdown into a Swiss minimal PDF.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/obk/cv-maker/examples"
	"github.com/obk/cv-maker/internal/config"
	"github.com/obk/cv-maker/internal/cv"
	"github.com/obk/cv-maker/internal/fonts"
	"github.com/obk/cv-maker/internal/render"
)

// version is set with -ldflags "-X main.version=..." by make; go install
// fills in the module version instead (see currentVersion).
var version = "dev"

func currentVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

const defaultInput = "cv.md"

const usage = `cvmaker turns a CV written in Markdown into a Swiss minimal PDF.

Usage:
  cvmaker [flags] [cv.md]         same as build
  cvmaker build [flags] [cv.md]   write the PDF next to the input
  cvmaker watch [flags] [cv.md]   rebuild whenever the CV, config or assets change
  cvmaker init  [dir]             create cv.md and config.yml to start from
  cvmaker version

  Without a file, cv.md in the current folder is used.

Flags for build and watch:
  -o <file>     output PDF path
  -c <file>     config file (default: config.yml or config.yaml next to cv.md)
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "cvmaker:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return buildCmd("build", nil)
	}
	switch cmd := args[0]; cmd {
	case "build", "watch":
		return buildCmd(cmd, args[1:])
	case "init":
		return initCmd(args[1:])
	case "version", "-v", "--version":
		fmt.Println("cvmaker", currentVersion())
		return nil
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	default:
		if strings.HasSuffix(strings.ToLower(cmd), ".md") || strings.HasPrefix(cmd, "-") {
			return buildCmd("build", args)
		}
		return fmt.Errorf("unknown command %q (see cvmaker help)", cmd)
	}
}

type job struct{ in, out, config string }

func buildCmd(cmd string, args []string) error {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	out := fs.String("o", "", "output PDF path")
	cfg := fs.String("c", "", "config file")
	// Allow flags after the file name too: cvmaker build cv.md -o out.pdf.
	var files []string
	for {
		if err := fs.Parse(args); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			break
		}
		files = append(files, fs.Arg(0))
		args = fs.Args()[1:]
	}
	switch len(files) {
	case 0:
		if !fileExists(defaultInput) {
			return fmt.Errorf("no %s in this folder; pass a Markdown file or run \"init\" to create one", defaultInput)
		}
		files = []string{defaultInput}
	case 1:
	default:
		return errors.New("expected one Markdown file")
	}
	j := job{in: files[0], out: *out, config: *cfg}
	if j.out == "" {
		j.out = strings.TrimSuffix(j.in, filepath.Ext(j.in)) + ".pdf"
	}
	if j.config == "" {
		dir := filepath.Dir(j.in)
		for _, name := range []string{"config.yml", "config.yaml"} {
			if fileExists(filepath.Join(dir, name)) {
				j.config = filepath.Join(dir, name)
				break
			}
		}
	}
	if cmd == "watch" {
		return watch(j)
	}
	if _, err := build(j); err != nil {
		return err
	}
	fmt.Println("wrote", j.out)
	return nil
}

// build renders one PDF and returns the files it read, so watch knows what
// to poll. The result is written to a temporary file first so a PDF viewer
// never sees a half written document.
func build(j job) ([]string, error) {
	files := []string{j.in}
	if j.config != "" {
		files = append(files, j.config)
	}
	src, err := os.ReadFile(j.in)
	if err != nil {
		return files, err
	}
	meta, body := config.Split(src)
	dir := filepath.Dir(j.in)
	cfg, err := config.Load(j.config, meta, dir)
	if err != nil {
		return files, err
	}
	for _, f := range []string{cfg.Font.Files.Regular, cfg.Font.Files.Bold, cfg.Font.Files.Italic, cfg.Font.Files.BoldItalic, cfg.Photo.Path} {
		if f != "" {
			files = append(files, f)
		}
	}
	set, err := fonts.Load(cfg.Font, dir)
	if err != nil {
		return files, err
	}

	var buf bytes.Buffer
	if err := render.PDF(&buf, cv.Parse(body, cfg.HardWraps), cfg, set); err != nil {
		return files, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(j.out), ".cvmaker-*.pdf")
	if err != nil {
		return files, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return files, err
	}
	if err := tmp.Close(); err != nil {
		return files, err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return files, err
	}
	return files, os.Rename(tmp.Name(), j.out)
}

// watch polls the inputs instead of using inotify and friends, which keeps
// the binary free of platform specific code.
func watch(j job) error {
	stamp := func(files []string) string {
		var b strings.Builder
		for _, f := range files {
			if st, err := os.Stat(f); err == nil {
				fmt.Fprintf(&b, "%s:%d:%d;", f, st.ModTime().UnixNano(), st.Size())
			}
		}
		return b.String()
	}
	fmt.Printf("watching %s (Ctrl+C to stop)\n", j.in)
	var last string
	var files []string
	for {
		if s := stamp(append(files, j.in)); s != last {
			var err error
			files, err = build(j)
			last = stamp(append(files, j.in))
			now := time.Now().Format("15:04:05")
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s error: %v\n", now, err)
			} else {
				fmt.Printf("%s wrote %s\n", now, j.out)
			}
		}
		time.Sleep(400 * time.Millisecond)
	}
}

func initCmd(args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	files := []struct {
		name string
		data []byte
	}{{"cv.md", examples.CV}, {"config.yml", examples.Config}}
	for _, f := range files {
		if path := filepath.Join(dir, f.name); fileExists(path) {
			return fmt.Errorf("%s already exists", path)
		}
	}
	for _, f := range files {
		path := filepath.Join(dir, f.name)
		if err := os.WriteFile(path, f.data, 0o644); err != nil {
			return err
		}
		fmt.Println("created", path)
	}
	fmt.Printf("next: cvmaker build %s\n", filepath.Join(dir, "cv.md"))
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
