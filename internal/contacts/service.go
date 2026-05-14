package contacts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"weview/internal/sqlitecli"
)

const (
	KindAll      = "all"
	KindFriend   = "friend"
	KindChatroom = "chatroom"
	KindOther    = "other"
)

type Contact struct {
	Username string `json:"username"`
	Alias    string `json:"alias"`
	Remark   string `json:"remark"`
	NickName string `json:"nick_name"`
	HeadURL  string `json:"head_url"`
	Kind     string `json:"kind"`
}

type Service struct {
	CacheDB string
}

func NewService(cacheDB string) Service {
	return Service{CacheDB: cacheDB}
}

func (s Service) List(ctx context.Context) ([]Contact, error) {
	if s.CacheDB == "" {
		return nil, fmt.Errorf("contact cache path is empty")
	}
	if _, err := os.Stat(s.CacheDB); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("contact cache does not exist: run `weview contacts --refresh` or `weview key` first")
		}
		return nil, err
	}

	query := `
SELECT
  username,
  COALESCE(local_type, 0) AS local_type,
  COALESCE(alias, '') AS alias,
  COALESCE(remark, '') AS remark,
  COALESCE(nick_name, '') AS nick_name,
  COALESCE(big_head_url, '') AS head_url
FROM contact
ORDER BY COALESCE(NULLIF(remark, ''), NULLIF(nick_name, ''), username) COLLATE NOCASE;
`
	cmd := exec.CommandContext(ctx, "sqlite3", "-json", sqlitecli.ImmutableURI(s.CacheDB), query)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("query contact cache: %v: %s", err, bytes.TrimSpace(out))
	}
	var rows []struct {
		Username  string `json:"username"`
		LocalType int    `json:"local_type"`
		Alias     string `json:"alias"`
		Remark    string `json:"remark"`
		NickName  string `json:"nick_name"`
		HeadURL   string `json:"head_url"`
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("parse sqlite json: %w", err)
	}
	contacts := make([]Contact, 0, len(rows))
	for _, row := range rows {
		contact := Contact{
			Username: row.Username,
			Alias:    row.Alias,
			Remark:   row.Remark,
			NickName: row.NickName,
			HeadURL:  row.HeadURL,
			Kind:     ClassifyKind(row.Username, row.LocalType),
		}
		contacts = append(contacts, contact)
	}
	return contacts, nil
}

func ClassifyKind(username string, localType int) string {
	if strings.HasSuffix(username, "@chatroom") {
		return KindChatroom
	}
	if localType == 1 && !strings.HasPrefix(username, "gh_") {
		return KindFriend
	}
	return KindOther
}

func FilterByKind(list []Contact, kind string) []Contact {
	if kind == "" || kind == KindAll {
		return list
	}
	out := make([]Contact, 0, len(list))
	for _, contact := range list {
		if contact.Kind == kind {
			out = append(out, contact)
		}
	}
	return out
}
