package contacts

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestListReturnsConfiguredContactFields(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 is required for contact query tests")
	}
	db := filepath.Join(t.TempDir(), "contact.db")
	sql := `
CREATE TABLE contact (username TEXT, local_type INTEGER, alias TEXT, remark TEXT, nick_name TEXT, big_head_url TEXT);
INSERT INTO contact VALUES ('u1', 1, 'alias1', 'Remark 1', 'Nick 1', 'https://example.com/1');
INSERT INTO contact VALUES ('10000@chatroom', 1, '', '', 'Group 1', '');
INSERT INTO contact VALUES ('gh_x', 1, '', '', 'OA', '');
`
	if out, err := exec.Command("sqlite3", db, sql).CombinedOutput(); err != nil {
		t.Fatalf("create db: %v: %s", err, out)
	}
	got, err := NewService(db).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d contacts, want 3", len(got))
	}

	byUser := map[string]Contact{}
	for _, c := range got {
		byUser[c.Username] = c
	}
	if byUser["u1"].Alias != "alias1" {
		t.Fatalf("u1 alias = %q", byUser["u1"].Alias)
	}
	if byUser["u1"].Remark != "Remark 1" {
		t.Fatalf("u1 remark = %q", byUser["u1"].Remark)
	}
	if byUser["10000@chatroom"].NickName != "Group 1" {
		t.Fatalf("chatroom nick_name = %q", byUser["10000@chatroom"].NickName)
	}
	if byUser["u1"].HeadURL != "https://example.com/1" {
		t.Fatalf("u1 head_url = %q", byUser["u1"].HeadURL)
	}
	if byUser["u1"].Kind != KindFriend {
		t.Fatalf("u1 kind = %q", byUser["u1"].Kind)
	}
	if byUser["10000@chatroom"].Kind != KindChatroom {
		t.Fatalf("chatroom kind = %q", byUser["10000@chatroom"].Kind)
	}
	if byUser["gh_x"].Kind != KindOther {
		t.Fatalf("gh_x kind = %q", byUser["gh_x"].Kind)
	}
}

func TestFilterByKind(t *testing.T) {
	list := []Contact{
		{Username: "u1", Kind: KindFriend},
		{Username: "g1@chatroom", Kind: KindChatroom},
		{Username: "gh_x", Kind: KindOther},
	}
	got := FilterByKind(list, KindChatroom)
	if len(got) != 1 || got[0].Username != "g1@chatroom" {
		t.Fatalf("filtered = %+v", got)
	}
	if len(FilterByKind(list, KindAll)) != 3 {
		t.Fatal("all kind should preserve list")
	}
}

func TestClassifyKind(t *testing.T) {
	tests := []struct {
		username  string
		localType int
		want      string
	}{
		{"wxid_a", 1, KindFriend},
		{"123@chatroom", 1, KindChatroom},
		{"123@chatroom", 2, KindChatroom},
		{"gh_x", 1, KindOther},
		{"wxid_room_member", 3, KindOther},
		{"corp_user", 5, KindOther},
	}
	for _, tt := range tests {
		if got := ClassifyKind(tt.username, tt.localType); got != tt.want {
			t.Fatalf("ClassifyKind(%q, %d) = %q, want %q", tt.username, tt.localType, got, tt.want)
		}
	}
}
