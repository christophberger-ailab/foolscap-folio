package main

import (
	"os"
	"regexp"
	"testing"
)

// GoReleaser applies a repository token as a lone environment variable,
// {{ .Env.NAME }}, without template functions (issue #108). Anything else
// fails the release when it publishes the Homebrew cask and the Scoop
// manifest.
func TestReleaseTokensAreSingleEnv(t *testing.T) {
	src, err := os.ReadFile("../../.goreleaser.yaml")
	if err != nil {
		t.Fatal(err)
	}
	tokens := regexp.MustCompile(`(?m)^\s*token:\s*(.*)$`).FindAllStringSubmatch(string(src), -1)
	if len(tokens) == 0 {
		t.Fatal("no token in .goreleaser.yaml")
	}
	single := regexp.MustCompile(`^["']?{{\s*\.Env\.[^.\s}]+\s*}}["']?$`)
	for _, m := range tokens {
		if !single.MatchString(m[1]) {
			t.Errorf("token %s is not a single {{ .Env.NAME }}", m[1])
		}
	}
}
