package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"

	"weview/internal/contacts"
)

func TestWriteContactsJSON(t *testing.T) {
	list := []contacts.Contact{{
		Username: "u1",
		Alias:    "a1",
		Remark:   "r1",
		NickName: "n1",
		HeadURL:  "https://example.com/1",
		Kind:     contacts.KindFriend,
	}}
	var out bytes.Buffer
	if err := writeContacts(&out, list, "json"); err != nil {
		t.Fatal(err)
	}
	var got []contacts.Contact
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].HeadURL != "https://example.com/1" || got[0].Kind != contacts.KindFriend {
		t.Fatalf("unexpected json output: %+v", got)
	}
}

func TestWriteContactsCSV(t *testing.T) {
	list := []contacts.Contact{{
		Username: "u1",
		Alias:    "alias,one",
		Remark:   "remark \"quoted\"",
		NickName: "nick\nname",
		HeadURL:  "https://example.com/1",
		Kind:     contacts.KindFriend,
	}}
	var out bytes.Buffer
	if err := writeContacts(&out, list, "csv"); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(out.String())).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d csv rows, want 2: %q", len(rows), out.String())
	}
	wantHeader := []string{"username", "alias", "remark", "nick_name", "head_url", "kind"}
	for i, want := range wantHeader {
		if rows[0][i] != want {
			t.Fatalf("header[%d] = %q, want %q", i, rows[0][i], want)
		}
	}
	if rows[1][1] != "alias,one" || rows[1][2] != `remark "quoted"` || rows[1][3] != "nick\nname" {
		t.Fatalf("csv row was not round-tripped correctly: %#v", rows[1])
	}
}

func TestWriteContactsJSONL(t *testing.T) {
	list := []contacts.Contact{
		{Username: "u1", NickName: "u1", Kind: contacts.KindFriend},
		{Username: "u2", NickName: "u2", Kind: contacts.KindChatroom},
	}
	var out bytes.Buffer
	if err := writeContacts(&out, list, "jsonl"); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d jsonl lines, want 2: %q", len(lines), out.String())
	}
	for _, line := range lines {
		var c contacts.Contact
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			t.Fatalf("invalid jsonl line %q: %v", line, err)
		}
	}
}

func TestWriteCount(t *testing.T) {
	var out bytes.Buffer
	if err := writeCount(&out, 12, "json"); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != `{"count":12}` {
		t.Fatalf("unexpected json count: %q", out.String())
	}

	out.Reset()
	if err := writeCount(&out, 12, "csv"); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(out.String())).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0][0] != "count" || rows[1][0] != "12" {
		t.Fatalf("unexpected csv count: %#v", rows)
	}

	out.Reset()
	if err := writeCount(&out, 12, "table"); err != nil {
		t.Fatal(err)
	}
	if out.String() != "12\n" {
		t.Fatalf("unexpected table count: %q", out.String())
	}
}
