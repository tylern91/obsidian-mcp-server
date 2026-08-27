package main

import (
	"bufio"
	"os"
	"strings"
	"testing"

	"github.com/tylern91/obsidian-mcp-server/internal/version"
)

func TestRunVersionFlag(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = origStdout }()

	runErr := run([]string{"--version"})

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	if runErr != nil {
		t.Fatalf("run([--version]) returned error: %v", runErr)
	}

	out, err := bufio.NewReader(r).ReadString('\n')
	if err != nil {
		t.Fatalf("read stdout pipe: %v", err)
	}
	want := "obsidian-mcp " + version.Version
	if strings.TrimSpace(out) != want {
		t.Errorf("stdout = %q, want %q", strings.TrimSpace(out), want)
	}
}
