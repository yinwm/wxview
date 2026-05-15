package media

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"wxview/internal/sqlitecli"
)

type voiceRow struct {
	DataHex string `json:"data_hex"`
}

type nameIDRow struct {
	ID int64 `json:"id"`
}

func (r Resolver) ResolveVoice(chatUsername string, localID int64, serverID int64, rawType int64) Info {
	if int64(uint64(rawType)&0xffffffff) != 34 {
		return notFound("voice", "not a voice message")
	}
	for _, dbPath := range r.ResourceDBs {
		if _, err := os.Stat(dbPath); err != nil {
			continue
		}
		if serverID > 0 {
			if data, ok := queryVoiceByServerID(dbPath, serverID); ok {
				return r.writeVoiceData(dbPath, data, fmt.Sprintf("svr:%d", serverID))
			}
		}
		if chatUsername != "" && localID > 0 {
			if chatID, ok := queryChatNameID(dbPath, chatUsername); ok {
				if data, ok := queryVoiceByLocalID(dbPath, chatID, localID); ok {
					return r.writeVoiceData(dbPath, data, fmt.Sprintf("chat:%d:%d", chatID, localID))
				}
			}
		}
	}
	return notFound("voice", "voice_data not found in media_*.db")
}

func (r Resolver) writeVoiceData(sourcePath string, data []byte, cacheKey string) Info {
	if len(data) == 0 {
		return notFound("voice", "empty voice_data")
	}
	out := data
	if bytes.HasPrefix(out, []byte("\x02#!SILK_V3")) {
		out = out[1:]
	}
	path, err := r.writeDecodedToCache(sourcePath, out, "silk", cacheKey)
	if err != nil {
		return decryptFailed("voice", sourcePath, err.Error(), false)
	}
	return Info{
		Kind:       "voice",
		Status:     "resolved",
		Path:       path,
		SourcePath: sourcePath,
		Decoded:    false,
		Thumbnail:  false,
		Reason:     "voice_data exported as silk",
	}
}

func queryVoiceByServerID(dbPath string, serverID int64) ([]byte, bool) {
	query := fmt.Sprintf("SELECT hex(voice_data) AS data_hex FROM VoiceInfo WHERE svr_id = %d AND length(voice_data) > 0 LIMIT 1;", serverID)
	return queryVoiceData(dbPath, query)
}

func queryVoiceByLocalID(dbPath string, chatNameID int64, localID int64) ([]byte, bool) {
	query := fmt.Sprintf("SELECT hex(voice_data) AS data_hex FROM VoiceInfo WHERE chat_name_id = %d AND local_id = %d AND length(voice_data) > 0 LIMIT 1;", chatNameID, localID)
	return queryVoiceData(dbPath, query)
}

func queryVoiceData(dbPath string, query string) ([]byte, bool) {
	cmd := exec.Command("sqlite3", "-json", sqlitecli.ImmutableURI(dbPath), query)
	out, err := cmd.CombinedOutput()
	if err != nil || len(bytes.TrimSpace(out)) == 0 {
		return nil, false
	}
	var rows []voiceRow
	if err := json.Unmarshal(out, &rows); err != nil || len(rows) == 0 {
		return nil, false
	}
	data, err := hex.DecodeString(strings.TrimSpace(rows[0].DataHex))
	return data, err == nil && len(data) > 0
}

func queryChatNameID(dbPath string, username string) (int64, bool) {
	query := fmt.Sprintf("SELECT rowid AS id FROM Name2Id WHERE user_name = %s LIMIT 1;", sqlLiteral(username))
	cmd := exec.Command("sqlite3", "-json", sqlitecli.ImmutableURI(dbPath), query)
	out, err := cmd.CombinedOutput()
	if err != nil || len(bytes.TrimSpace(out)) == 0 {
		return 0, false
	}
	var rows []nameIDRow
	if err := json.Unmarshal(out, &rows); err != nil || len(rows) == 0 {
		return 0, false
	}
	return rows[0].ID, rows[0].ID > 0
}
