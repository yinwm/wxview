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
		"--format csv",
		"--kind friend",
		"--query TEXT",
		"--limit N",
		"--count",
		"Output fields",
		"Examples for AI/tools",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("contacts help missing %q:\n%s", want, text)
		}
	}
}

func TestDaemonHelpShowsStatusAndNoQueryAction(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"daemon", "--help"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{
		"Supported forms",
		"weview daemon         Show this help.",
		"weview daemon start",
		"weview daemon stop",
		"weview daemon status",
		"No daemon flags are currently supported",
		"health",
		"refresh_contacts",
		"stop",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("daemon help missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "list_contacts") {
		t.Fatalf("daemon help should not advertise list_contacts:\n%s", text)
	}
}

func TestDaemonWithoutArgsShowsHelp(t *testing.T) {
	t.Setenv(daemonForegroundEnv, "")
	var out bytes.Buffer
	if err := run([]string{"daemon"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	var help bytes.Buffer
	if err := run([]string{"daemon", "--help"}, &help, &help); err != nil {
		t.Fatal(err)
	}
	if out.String() != help.String() {
		t.Fatalf("daemon without args should match daemon --help\nwithout args:\n%s\nhelp:\n%s", out.String(), help.String())
	}
}

func TestUnknownDaemonSubcommandIsRejected(t *testing.T) {
	var out bytes.Buffer
	err := run([]string{"daemon", "run"}, &out, &out)
	if err == nil {
		t.Fatal("expected unknown daemon subcommand to be rejected")
	}
	if !strings.Contains(err.Error(), "unknown daemon command: run") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnexpectedDaemonStartArgumentIsRejected(t *testing.T) {
	var out bytes.Buffer
	err := run([]string{"daemon", "start", "extra"}, &out, &out)
	if err == nil {
		t.Fatal("expected extra daemon start argument to be rejected")
	}
	if !strings.Contains(err.Error(), "unexpected daemon start argument: extra") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnexpectedDaemonStopArgumentIsRejected(t *testing.T) {
	var out bytes.Buffer
	err := run([]string{"daemon", "stop", "extra"}, &out, &out)
	if err == nil {
		t.Fatal("expected extra daemon stop argument to be rejected")
	}
	if !strings.Contains(err.Error(), "unexpected daemon stop argument: extra") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnexpectedDaemonStatusArgumentIsRejected(t *testing.T) {
	var out bytes.Buffer
	err := run([]string{"daemon", "status", "extra"}, &out, &out)
	if err == nil {
		t.Fatal("expected extra daemon status argument to be rejected")
	}
	if !strings.Contains(err.Error(), "unexpected daemon status argument: extra") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestContactsWithoutArgsShowsHelp(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"contacts"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{
		"weview contacts - List WeChat contacts",
		"No-argument behavior",
		"weview contacts is intentionally the same as weview contacts --help",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("contacts no-arg help missing %q:\n%s", want, text)
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

func TestKeyCommandIsNotAccepted(t *testing.T) {
	var out bytes.Buffer
	err := run([]string{"key"}, &out, &out)
	if err == nil {
		t.Fatal("expected key command to be rejected")
	}
	if strings.Contains(out.String(), "sudo weview key") {
		t.Fatalf("key command should not be advertised:\n%s", out.String())
	}
}
