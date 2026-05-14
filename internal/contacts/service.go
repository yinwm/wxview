package contacts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

type QueryOptions struct {
	Kind     string
	Query    string
	Username string
	Sort     string
	Limit    int
	Offset   int
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
			return nil, fmt.Errorf("contact cache does not exist: run `weview contacts --refresh` or `weview init` first")
		}
		return nil, err
	}

	query := `
SELECT
  COALESCE(id, 0) AS id,
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
		ID        int    `json:"id"`
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
	ownerUsername := ownerUsernameFromCachePath(s.CacheDB)
	contacts := make([]Contact, 0, len(rows))
	for _, row := range rows {
		contact := Contact{
			Username: row.Username,
			Alias:    row.Alias,
			Remark:   row.Remark,
			NickName: row.NickName,
			HeadURL:  row.HeadURL,
			Kind:     ClassifyKindForAccount(row.Username, row.LocalType, row.ID, ownerUsername),
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

func ClassifyKindForAccount(username string, localType int, contactID int, ownerUsername string) string {
	if isSelfContact(username, contactID, ownerUsername) {
		return KindOther
	}
	return ClassifyKind(username, localType)
}

func isSelfContact(username string, contactID int, ownerUsername string) bool {
	if ownerUsername != "" && username == ownerUsername {
		return true
	}
	return contactID == 2 && strings.HasPrefix(username, "wxid_")
}

func ownerUsernameFromCachePath(cacheDB string) string {
	account := filepath.Base(filepath.Dir(filepath.Dir(cacheDB)))
	if !strings.HasPrefix(account, "wxid_") {
		return account
	}
	idx := strings.LastIndex(account, "_")
	if idx <= len("wxid_") {
		return account
	}
	candidate := account[:idx]
	if strings.HasPrefix(candidate, "wxid_") {
		return candidate
	}
	return account
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

func ApplyQueryOptions(list []Contact, opts QueryOptions) []Contact {
	filtered := make([]Contact, 0, len(list))
	query := strings.ToLower(strings.TrimSpace(opts.Query))
	username := strings.TrimSpace(opts.Username)
	for _, contact := range list {
		if opts.Kind != "" && opts.Kind != KindAll && contact.Kind != opts.Kind {
			continue
		}
		if username != "" && contact.Username != username {
			continue
		}
		if query != "" && !matchesQuery(contact, query) {
			continue
		}
		filtered = append(filtered, contact)
	}
	sortContacts(filtered, opts.Sort)
	return paginate(filtered, opts.Limit, opts.Offset)
}

func matchesQuery(contact Contact, query string) bool {
	for _, value := range []string{contact.Username, contact.Alias, contact.Remark, contact.NickName} {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}

func sortContacts(list []Contact, sortBy string) {
	switch sortBy {
	case "", "username":
		sort.SliceStable(list, func(i, j int) bool {
			return strings.ToLower(list[i].Username) < strings.ToLower(list[j].Username)
		})
	case "name":
		sort.SliceStable(list, func(i, j int) bool {
			left := strings.ToLower(DisplayName(list[i]))
			right := strings.ToLower(DisplayName(list[j]))
			if left == right {
				return strings.ToLower(list[i].Username) < strings.ToLower(list[j].Username)
			}
			return left < right
		})
	}
}

func DisplayName(contact Contact) string {
	if strings.TrimSpace(contact.Remark) != "" {
		return contact.Remark
	}
	if strings.TrimSpace(contact.NickName) != "" {
		return contact.NickName
	}
	if strings.TrimSpace(contact.Alias) != "" {
		return contact.Alias
	}
	return contact.Username
}

func paginate(list []Contact, limit int, offset int) []Contact {
	if offset < 0 {
		offset = 0
	}
	if limit < 0 {
		limit = 0
	}
	if offset >= len(list) {
		return []Contact{}
	}
	if limit == 0 {
		return list[offset:]
	}
	end := offset + limit
	if end > len(list) {
		end = len(list)
	}
	return list[offset:end]
}
