package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootHelpIsDescriptive(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"--help"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{
		"weview - Read local WeChat data",
		"Machine-readable usage",
		"weview init",
		"weview contacts --help",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("root help missing %q:\n%s", want, text)
		}
	}
}

func TestContactsHelpIsActionable(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"contacts", "--help"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{
		"--format json",
		"--kind friend",
		"Output fields",
		"Examples for AI/tools",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("contacts help missing %q:\n%s", want, text)
		}
	}
}

func TestContractHelpTypoAlias(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"contract", "--help"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "official command is `contacts`") {
		t.Fatalf("expected typo alias note:\n%s", out.String())
	}
}
