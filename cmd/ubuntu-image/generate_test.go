package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra/doc"
)

/*
 * using test file for manpage and bash completion generate so that
 * we don't embed the code and dependencies in final binary
 */
var generate = flag.Bool("generate", false, "generate manpages and completion files")
var out = flag.String("path", "build", "custom directory where to generate files when using --generate")

func TestGenerateManpage(t *testing.T) {
	t.Parallel()

	if err := os.Mkdir(*out, 0755); err != nil && os.IsNotExist(err) {
		t.Fatalf("couldn't create %s directory: %v", *out, err)
	}
	header := &doc.GenManHeader{
		Title:   "UBUNTU-IMAGE",
		Section: "1",
	}
	if err := doc.GenManTree(generateRootCmd(), header, *out); err != nil {
		t.Fatalf("couldn't generate manpage: %v", err)
	}
}

func TestGenerateCompletion(t *testing.T) {
	t.Parallel()

	rootCmd := generateRootCmd()
	if err := os.Mkdir(*out, 0755); err != nil && os.IsNotExist(err) {
		t.Fatalf("couldn't create %s directory: %v", *out, err)
	}
	if err := rootCmd.GenBashCompletionFile(filepath.Join(*out, "bash-completion")); err != nil {
		t.Fatalf("couldn't generate bash completion: %v", err)
	}
	if err := rootCmd.GenZshCompletionFile(filepath.Join(*out, "zsh-completion")); err != nil {
		t.Fatalf("couldn't generate bazshsh completion: %v", err)
	}
}

//TODO: cobra can also generate the README.md. Do that.
