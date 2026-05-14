package main

import (
	"bytes"
	"strings"
	"testing"

	"weview/internal/key"
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
		"weview messages --help",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("root help missing %q:\n%s", want, text)
		}
	}
}

func TestInitOutputDefaultsToSummary(t *testing.T) {
	results := []key.EnsureResult{
		{
			Entry:  key.Entry{Fingerprint: "abc"},
			Target: key.TargetDB{Account: "wxid_a", DataDir: "/tmp/db_storage", DBRelPath: "contact/contact.db"},
			Reused: false,
		},
		{
			Entry:  key.Entry{Fingerprint: "def"},
			Target: key.TargetDB{Account: "wxid_a", DataDir: "/tmp/db_storage", DBRelPath: "message/message_0.db"},
			Reused: true,
		},
	}
	warnings := []key.EnsureWarning{{
		DBRelPath: "message/message_revoke.db",
		Message:   "auxiliary key missing; skipped. Retry later with `sudo weview init`.",
	}}

	var out bytes.Buffer
	if err := writeInitOutput(&out, results, warnings, false); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{
		"account: wxid_a",
		"data_dir: /tmp/db_storage",
		"keys_total: 2",
		"keys_scanned: 1",
		"keys_reused: 1",
		"warnings_total: 1",
		"message/message_revoke.db auxiliary key missing",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("init summary missing %q:\n%s", want, text)
		}
	}
	for _, notWant := range []string{
		"\nkeys:\n",
		"fingerprint=",
		"contact/contact.db fingerprint",
	} {
		if strings.Contains(text, notWant) {
			t.Fatalf("init summary should not contain %q:\n%s", notWant, text)
		}
	}
}

func TestInitOutputVerboseIncludesKeyDetails(t *testing.T) {
	results := []key.EnsureResult{{
		Entry:  key.Entry{Fingerprint: "abc"},
		Target: key.TargetDB{Account: "wxid_a", DataDir: "/tmp/db_storage", DBRelPath: "contact/contact.db"},
		Reused: false,
	}}

	var out bytes.Buffer
	if err := writeInitOutput(&out, results, nil, true); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{
		"keys:",
		"contact/contact.db fingerprint=abc status=scanned",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("verbose init output missing %q:\n%s", want, text)
		}
	}
}

func TestMessagesHelpIsActionable(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"messages", "--help"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{
		"weview messages - List WeChat messages",
		"--username TEXT",
		"--start TIME",
		"--end TIME",
		"--after-seq N",
		"--limit N",
		"--offset N",
		"--source",
		"create_time ascending",
		"message/message_*.db",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("messages help missing %q:\n%s", want, text)
		}
	}
}

func TestMessagesWithoutArgsShowsHelp(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"messages"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{
		"weview messages - List WeChat messages",
		"Required:",
		"--username TEXT",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("messages no-arg help missing %q:\n%s", want, text)
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
		"refresh_messages",
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
