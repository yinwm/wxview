package main

import (
	"bytes"
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
