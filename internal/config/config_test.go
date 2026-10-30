package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSplit(t *testing.T) {
	meta, body := Split([]byte("---\npage: Letter\n---\n# Name\n"))
	if string(meta) != "page: Letter\n" || string(body) != "# Name\n" {
		t.Fatalf("got meta %q body %q", meta, body)
	}
	meta, body = Split([]byte("# Name\n---\n"))
	if meta != nil || string(body) != "# Name\n---\n" {
		t.Fatalf("no front matter: got meta %q body %q", meta, body)
	}
}

func TestLoadLayers(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	os.WriteFile(cfgPath, []byte("font:\n  size: 11\nphoto:\n  path: me.jpg\n"), 0o644)

	cfg, err := Load(cfgPath, []byte("font:\n  name_size: 30\n"), "/elsewhere")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Font.Size != 11 || cfg.Font.NameSize != 30 || cfg.Font.LineHeight != Default().Font.LineHeight {
		t.Errorf("font settings not layered: %+v", cfg.Font)
	}
	if want := filepath.Join(dir, "me.jpg"); cfg.Photo.Path != want {
		t.Errorf("photo path = %q, want %q", cfg.Photo.Path, want)
	}
}

func TestFilesReplaceFamily(t *testing.T) {
	cfg, err := Load("", []byte("font:\n  files: {regular: a.ttf, bold: b.ttf, italic: c.ttf, bold_italic: d.ttf}\n"), "/cv")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Font.Family != "" || cfg.Font.Files.Regular != "/cv/a.ttf" {
		t.Errorf("got %+v", cfg.Font)
	}
}

func TestValidate(t *testing.T) {
	for _, meta := range []string{
		"page: A5",
		"colors: {text: red}",
		"font: {size: 0}",
		"photo: {shape: star}",
		"font: {files: {regular: a.ttf}}",
		"unknown_key: 1",
	} {
		if _, err := Load("", []byte(meta), "."); err == nil {
			t.Errorf("%q: expected an error", meta)
		}
	}
}

func TestParseColor(t *testing.T) {
	c, err := ParseColor("#3a3")
	if err != nil || c != (RGB{0x33, 0xaa, 0x33}) {
		t.Fatalf("got %v, %v", c, err)
	}
}
