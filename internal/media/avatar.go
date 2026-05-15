package media

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"wxview/internal/sqlitecli"
)

type avatarRow struct {
	ImageHex string `json:"image_hex"`
}

func ResolveAvatar(headImageDB string, username string, cacheDir string) Info {
	if headImageDB == "" || username == "" {
		return notFound("avatar", "head_image cache or username is empty")
	}
	if _, err := os.Stat(headImageDB); err != nil {
		return notFound("avatar", "head_image cache not found")
	}
	query := fmt.Sprintf("SELECT hex(image_buffer) AS image_hex FROM head_image WHERE username = %s AND length(image_buffer) > 0 LIMIT 1;", sqlLiteral(username))
	cmd := exec.Command("sqlite3", "-json", sqlitecli.ImmutableURI(headImageDB), query)
	out, err := cmd.CombinedOutput()
	if err != nil || len(bytes.TrimSpace(out)) == 0 {
		return notFound("avatar", "local avatar not found in head_image.db")
	}
	var rows []avatarRow
	if err := json.Unmarshal(out, &rows); err != nil || len(rows) == 0 {
		return notFound("avatar", "local avatar not found in head_image.db")
	}
	data, err := hex.DecodeString(rows[0].ImageHex)
	if err != nil || len(data) == 0 {
		return notFound("avatar", "local avatar image buffer is empty")
	}
	resolver := Resolver{CacheDir: cacheDir}
	ext := imageExt(data)
	path, err := resolver.writeDecodedToCache(headImageDB, data, ext, "avatar:"+username)
	if err != nil {
		return decryptFailed("avatar", headImageDB, err.Error(), false)
	}
	info := resolved("avatar", path, headImageDB, false, false)
	if ext != "gif" {
		info = withImageDimensions(info)
	}
	return info
}

func imageExt(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}):
		return "jpg"
	case bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G'}):
		return "png"
	case bytes.HasPrefix(data, []byte("GIF87a")), bytes.HasPrefix(data, []byte("GIF89a")):
		return "gif"
	default:
		return "jpg"
	}
}
