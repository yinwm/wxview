package media

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"weview/internal/sqlitecli"
)

type resourceMediaFile struct {
	Key        string
	Name       string
	Size       int64
	ModifyTime int64
	Path       string
}

type resourceMediaRow struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	ModifyTime   int64  `json:"modify_time"`
	Dir1         string `json:"dir1"`
	Dir2         string `json:"dir2"`
	RelativePath string `json:"relative_path"`
}

func queryResourceMedia(dbPaths []string, dataDir string, mediaType string, keys []string) []resourceMediaFile {
	keys = cleanMediaKeys(keys)
	if len(keys) == 0 {
		return nil
	}
	var out []resourceMediaFile
	for _, dbPath := range dbPaths {
		if _, err := os.Stat(dbPath); err != nil {
			continue
		}
		out = append(out, queryHardlinkMedia(dbPath, dataDir, mediaType, keys)...)
		out = append(out, queryHlinkMedia(dbPath, dataDir, keys)...)
	}
	return dedupeResourceFiles(out)
}

func queryHardlinkMedia(dbPath string, dataDir string, mediaType string, keys []string) []resourceMediaFile {
	tablePrefix := ""
	switch mediaType {
	case "image":
		tablePrefix = "image"
	case "video":
		tablePrefix = "video"
	case "file":
		tablePrefix = "file"
	default:
		return nil
	}
	accountBase := filepath.Dir(dataDir)
	condition := resourceKeyCondition(keys, "f.md5", "f.file_name")
	var out []resourceMediaFile
	for _, table := range []string{tablePrefix + "_hardlink_info_v3", tablePrefix + "_hardlink_info_v4"} {
		query := fmt.Sprintf(`
SELECT
  COALESCE(f.md5, '') AS key,
  COALESCE(f.file_name, '') AS name,
  COALESCE(f.file_size, 0) AS size,
  COALESCE(f.modify_time, 0) AS modify_time,
  COALESCE(d1.username, '') AS dir1,
  COALESCE(d2.username, '') AS dir2,
  '' AS relative_path
FROM %s f
LEFT JOIN dir2id d1 ON d1.rowid = f.dir1
LEFT JOIN dir2id d2 ON d2.rowid = f.dir2
WHERE %s;
`, table, condition)
		rows := queryResourceRows(dbPath, query)
		for _, row := range rows {
			path := hardlinkPath(accountBase, mediaType, row)
			if path == "" {
				continue
			}
			out = append(out, resourceMediaFile{
				Key:        row.Key,
				Name:       row.Name,
				Size:       row.Size,
				ModifyTime: row.ModifyTime,
				Path:       path,
			})
		}
	}
	return out
}

func queryHlinkMedia(dbPath string, dataDir string, keys []string) []resourceMediaFile {
	accountBase := filepath.Dir(dataDir)
	condition := resourceKeyCondition(keys, "r.mediaMd5", "d.fileName")
	query := fmt.Sprintf(`
SELECT
  COALESCE(r.mediaMd5, '') AS key,
  COALESCE(d.fileName, '') AS name,
  COALESCE(r.mediaSize, 0) AS size,
  COALESCE(r.modifyTime, 0) AS modify_time,
  '' AS dir1,
  '' AS dir2,
  COALESCE(d.relativePath, '') AS relative_path
FROM HlinkMediaRecord r
JOIN HlinkMediaDetail d ON r.inodeNumber = d.inodeNumber
WHERE %s;
`, condition)
	rows := queryResourceRows(dbPath, query)
	out := make([]resourceMediaFile, 0, len(rows))
	for _, row := range rows {
		if row.RelativePath == "" || row.Name == "" {
			continue
		}
		out = append(out, resourceMediaFile{
			Key:        row.Key,
			Name:       row.Name,
			Size:       row.Size,
			ModifyTime: row.ModifyTime,
			Path:       filepath.Join(accountBase, "Message", "MessageTemp", filepath.FromSlash(row.RelativePath), row.Name),
		})
	}
	return out
}

func queryResourceRows(dbPath string, query string) []resourceMediaRow {
	cmd := exec.Command("sqlite3", "-json", sqlitecli.ImmutableURI(dbPath), query)
	out, err := cmd.CombinedOutput()
	if err != nil || len(bytes.TrimSpace(out)) == 0 {
		return nil
	}
	var rows []resourceMediaRow
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil
	}
	return rows
}

func hardlinkPath(accountBase string, mediaType string, row resourceMediaRow) string {
	if row.Name == "" {
		return ""
	}
	switch mediaType {
	case "image":
		if row.Dir1 == "" || row.Dir2 == "" {
			return ""
		}
		return filepath.Join(accountBase, "msg", "attach", row.Dir1, row.Dir2, "Img", row.Name)
	case "video":
		if row.Dir1 == "" {
			return ""
		}
		return filepath.Join(accountBase, "msg", "video", row.Dir1, row.Name)
	case "file":
		if row.Dir1 == "" {
			return ""
		}
		return filepath.Join(accountBase, "msg", "file", row.Dir1, row.Name)
	default:
		return ""
	}
}

func resourceKeyCondition(keys []string, keyColumn string, nameColumn string) string {
	parts := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		quoted := sqlLiteral(key)
		parts = append(parts, keyColumn+" = "+quoted)
		parts = append(parts, nameColumn+" LIKE "+quoted+" || '%'")
	}
	return strings.Join(parts, " OR ")
}

func cleanMediaKeys(keys []string) []string {
	var out []string
	for _, key := range keys {
		key = strings.ToLower(strings.TrimSpace(key))
		if len(key) < 6 {
			continue
		}
		valid := true
		for _, r := range key {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
				continue
			}
			valid = false
			break
		}
		if valid && !contains(out, key) {
			out = append(out, key)
		}
	}
	return out
}

func sqlLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func dedupeResourceFiles(files []resourceMediaFile) []resourceMediaFile {
	seen := map[string]bool{}
	out := make([]resourceMediaFile, 0, len(files))
	for _, file := range files {
		if file.Path == "" || seen[file.Path] {
			continue
		}
		seen[file.Path] = true
		out = append(out, file)
	}
	return out
}
