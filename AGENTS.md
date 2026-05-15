# Agent Notes for Weview

Weview is a local-first CLI for reading data from the user's own macOS WeChat.
The V1 implementation is intentionally narrow: initialize supported DB keys,
decrypt `contact/contact.db`, `session/session.db`, message-related DBs, and
selected optional data DBs, run a local daemon for cache maintenance, list
contacts/groups from the decrypted contact cache, query recent/unread/incremental
sessions, and query history for an explicit username from decrypted
`message/message_[number].db` caches. It also supports bounded message-content
`search` and cross-conversation `timeline` queries for AI and script consumers.
Image, video, file, voice, and local avatar media should resolve usable local
paths automatically when the supporting caches are available.

## Collaboration Rules

- Do not run git operations such as commit, push, rebase, or branch changes
  unless the user explicitly asks for git work.
- Do not print full DB keys, raw secrets, or unnecessary WeChat data. Printing a
  short key fingerprint is acceptable.
- `wechat-decrypt/` is an external reference repo and is ignored by the root git
  repo. Read it for behavior, but do not mix it into the Go runtime path.
- Runtime state under `~/.weview/` is local user data and must not be committed.

## Command Semantics

- `weview init` is the first-time setup command. It detects WeChat, finds or
  reuses required DB keys, verifies each against page 1 HMAC, and saves them
  in `~/.weview/cache/<account>/keys.json`. Normal use should run it once at
  the beginning. Auxiliary message DB keys are best-effort and should warn
  rather than block init.
- `weview init` should be concise by default: print account, data_dir, key
  counts, and warnings. Per-DB fingerprints/status belong behind `--verbose`.
- Current account detection should prefer the account whose `db_storage` files
  are currently open by the running WeChat process. File mtime is only a
  fallback when open-file detection cannot identify an account.
- `weview contacts` with no flags must show the same help as
  `weview contacts --help`; it should not query data. Require an explicit
  output/filter flag such as `--format` or `--kind` to query.
- `weview contact` is a supported alias for `weview contacts`.
- `weview contract --help` is accepted only as a typo-friendly help alias. Do
  not make `contract` an official command.
- `weview contacts` is intended to be usable by other tools and AI agents. It
  supports `--format table|json|jsonl|csv`, `--kind`, `--query`, `--username`,
  `--limit`, `--offset`, `--sort username|name`, and `--count`. Prefer `json`,
  `jsonl`, or `csv` plus an explicit `--limit` for automated reads.
- `--count` reports the filtered total before pagination; `--limit` and
  `--offset` do not affect the count.
- `weview messages` requires `--username`; without args it must show the same
  help as `weview messages --help`. It supports `--format table|json|jsonl|csv`,
  `--date`, `--start`, `--end`, `--after-seq`, `--limit`, `--offset`,
  `--source`, and `--refresh`.
- `weview messages --format json` returns a `{meta, items}` envelope. `meta`
  includes `schema_version` and `timezone`. Reliable machine pagination should
  follow `meta.next_args` / `meta.next_after_seq`; `--offset` is mainly for
  temporary human reads. `jsonl`, `csv`, and `table` remain item-only outputs.
- Treat the `--username` value as an ordinary chat target even when it matches
  the current account username. Do not add a special self-chat guard or require
  an extra override flag for that case.
- `weview messages` returns records sorted by time ascending by default. Apply
  `--limit` and `--offset` after merging all matching message shards.
- V1 does not maintain a message index. Large chats or broad time ranges can be
  slow because message shards are merged before pagination.
- `--start` and `--end` are inclusive. Date-only `--end` includes the full day.
- `messages --date today|yesterday|YYYY-MM-DD` selects one local calendar day
  and is mutually exclusive with `--start`/`--end`.
- `--after-seq` is a cursor-style filter. It returns only rows whose `seq` is
  greater than the provided value, then global sorting and pagination still
  apply.
- `--source` is a debugging flag only. It may expose source DB/table/local row
  metadata for cache/shard diagnosis, but normal chat-history reads should not
  require it.
- `weview timeline` selects one or more conversations and returns a bounded
  cross-conversation time range. It supports `--format table|json|jsonl|csv`,
  `--kind all|friend|chatroom|other`, `--query`, `--username`, `--date`,
  `--start`, `--end`, `--limit`, `--cursor`, `--source`, and `--refresh`.
- V1 must not imply `timeline` is indexed. It fans out across selected
  conversations and message shards, merges results in-process, then applies the
  global limit. Broad selectors or wide date ranges can be very slow; prefer
  narrow selectors and small time windows in examples and AI/tool usage.
- `weview timeline` must have an explicit time range: either `--date
  today|yesterday|YYYY-MM-DD` or both `--start` and `--end`. `--date` is
  mutually exclusive with `--start`/`--end`.
- `weview timeline` must have an explicit conversation selector: either
  `--username`, or `--kind`/`--query`. `--username` cannot be combined with
  `--kind` or `--query`.
- `timeline --query` filters conversation metadata (`username`, `alias`,
  `remark`, `nick_name`); it does not search message content.
- `weview timeline --format json` returns a `{meta, items}` envelope with
  `meta.schema_version`, `meta.timezone`, `meta.next_cursor`, and
  `meta.next_args`. Reliable pagination consumers should follow
  `meta.next_args`; `jsonl`, `csv`, and `table` remain item-only outputs.
- `timeline` `matched_chats` is the number of conversations selected by the
  filter, not the number of conversations that have returned messages on the
  current page or within the requested time range.
- `weview search` searches message content and parsed `content_detail` values.
  The initial implementation is a local scan over selected conversations; future
  FTS/index work should be treated as an accelerator, not as part of the item
  schema contract. JSON output uses the same `{meta, items}` envelope and message
  item schema as `messages` and `timeline`.
- `weview sessions`, `weview unread`, and `weview new-messages` read
  `session/session.db` directly from the local decrypted cache. `new-messages`
  keeps account-scoped state under `~/.weview/cache/<account>/state/`.
- Future performance work should be documented as optional TODOs unless the
  product direction explicitly changes. Likely directions are SQL pagination
  pushdown for `messages`, `timeline --explain` / dry-run estimation, batched
  timeline fan-out with stable cursor semantics, and an optional local message
  index. A future FTS5 index should be treated mainly as a `search` accelerator;
  `messages` and `timeline` need ordinary chat/time indexes to become fast.
- Message `items` from `messages`, `timeline`, `search`, and `new-messages` must
  share the same schema.
  Include chat metadata fields `chat_kind`, `chat_display_name`, `chat_alias`,
  `chat_remark`, and `chat_nick_name`. `chat_display_name` is `remark >
  nick_name > alias > username`; missing contacts fall back to `chat_username`
  and `chat_kind=unknown`.
- `content` must remain the raw decoded message body. Use `content_detail` for
  convenience parsing. For image and video messages, parse useful XML metadata
  such as md5, length/play length, thumbnail dimensions, and CDN file
  identifiers there. Do not copy CDN AES keys into `content_detail`.
- `weview messages` resolves image, video, file, and voice usability automatically for
  returned rows. It should resolve local media files from WeChat storage and
  decode supported `.dat` media into `~/.weview/cache/<account>/media/`.
  Put the result in `content_detail`, not a separate top-level `media` object:
  include `media_status`, `path`, `source_path`, `decoded`, `thumbnail`,
  `thumbnail_path`, `thumbnail_source_path`, `thumbnail_decoded`, `width`,
  `height`, and `media_reason` when available.
- `weview messages` must not scan WeChat process memory. If a message key is
  missing or invalid, tell the user to run `sudo weview init`.

## Contact Output Contract

The current `contacts` output fields are:

- `username`: stable WeChat username, such as `wxid_*` or `*@chatroom`
- `alias`: WeChat ID / alias when present
- `remark`: user's remark for the contact
- `nick_name`: nickname from WeChat
- `head_url`: avatar image URL
- `kind`: one of `friend`, `chatroom`, or `other`

The `kind` classifier is intentionally pragmatic:

- `friend`: ordinary private-chat contacts
- `chatroom`: usernames ending in `@chatroom`
- `other`: official accounts, enterprise contacts, non-friend room members, and
  special/system contacts
- `all`: CLI filter value only, not an output kind

Avoid reintroducing `is_friend`; its semantics were too ambiguous for the
product goal.

## Daemon and Cache

- V1 does not patch WAL files. Refresh is near-real-time after WeChat writes or
  checkpoints the main DB.
- In V1, the daemon is a cache maintenance service, not a contacts query
  service. It should focus on cache refresh, source DB watching, `health`,
  `refresh_contacts`, `refresh_sessions`, `refresh_messages`,
  `refresh_avatars`, `refresh_favorites`, `refresh_sns`, and `stop`.
- The only supported daemon CLI forms are `weview daemon`, `weview daemon start`,
  `weview daemon stop`, and `weview daemon status`. Bare `weview daemon` must
  show the same help as `weview daemon --help` and must not start the daemon.
- `weview contacts ...` should always read contacts directly from the local
  decrypted cache, then apply filtering, sorting, pagination, counts, and output
  formatting in the CLI path.
- If `contacts --refresh` is used and the daemon is running, the CLI may ask the
  daemon to refresh first, then still read the cache directly. If the daemon is
  not running, the CLI may refresh the cache in-process.
- `weview messages ...` should always read messages directly from local
  decrypted message caches. If `--refresh` is used and the daemon is running,
  it may ask the daemon to refresh message caches first; otherwise it may
  refresh message caches in-process. Do not route message queries through the
  daemon in V1.
- `weview sessions ...` and `weview unread ...` should always read sessions
  directly from the local decrypted session cache. If `--refresh` is used and
  the daemon is running, they may ask the daemon to refresh session cache first;
  otherwise they may refresh session cache in-process.
- `weview new-messages ...` should use `session/session.db` as the changed-chat
  index, then read matching message rows directly from local decrypted message
  caches. Do not route new-message queries through the daemon in V1.
- `weview favorites ...` should always read favorites directly from the local
  decrypted favorite cache. If `--refresh` is used and the daemon is running,
  it may ask the daemon to refresh favorite cache first; otherwise it may
  refresh favorite cache in-process.
- `weview sns ...` should always read SNS data directly from the local
  decrypted SNS cache. If `--refresh` is used and the daemon is running, it may
  ask the daemon to refresh SNS cache first; otherwise it may refresh SNS cache
  in-process.
- `weview timeline ...` should select conversations from the local decrypted
  contact cache and read messages directly from local decrypted message caches.
  If `--refresh` is used, refresh both contact and message caches, preferring
  daemon refresh actions when available. Do not route timeline queries through
  the daemon in V1.
- Do not reintroduce daemon-side `list_contacts` unless the product direction
  explicitly changes to a real query service, Web API, or MCP service.
- The daemon uses the internal Unix socket `~/.weview/weview.sock`. Treat this as
  internal transport, not a public Web API.
- Decrypted contact cache path is:
  `~/.weview/cache/<account>/contact/contact.db`
- Decrypted message cache paths are:
  `~/.weview/cache/<account>/message/message_*.db`
- Decrypted session cache path is:
  `~/.weview/cache/<account>/session/session.db`
- Decrypted head image cache path is:
  `~/.weview/cache/<account>/head_image/head_image.db`
- Account key store path is:
  `~/.weview/cache/<account>/keys.json`. Do not use a global
  `~/.weview/keys.json`; keys are account-scoped.
- Cache refresh metadata path is:
  `~/.weview/cache/<account>/mtime.json`. It records source DB path, size,
  mtime, salt, cache path, and refresh time. Refresh should skip decrypting a
  DB when metadata still matches and the cache file exists.
- Daemon watchers should resolve the current account dynamically, so logging out
  of one WeChat account and logging into another while the daemon is running
  switches to the new account's key/cache metadata.
- Supported message-related DBs include numeric正文分片
  `message/message_[number].db` plus `message/biz_message_[number].db`,
  `message/media_[number].db`, `message/message_fts.db`,
  `message/message_resource.db`, and `message/message_revoke.db`.
- Required DB keys are `contact/contact.db` plus numeric message shards.
  `session/session.db`, `head_image/head_image.db`, `message/media_*.db`,
  `message_fts.db`, `message_resource.db`, and `message_revoke.db` are
  auxiliary/optional and should not block init/cache refresh when their key is
  not yet present in WeChat process memory.
- Use readable account directory names, not base64, unless a future account name
  contains unsafe path characters. Unsafe characters should be replaced with `_`.
- When running under `sudo`, state written under `~/.weview` should be chowned
  back to the original user from `SUDO_UID`/`SUDO_GID`.

## SQLite and Decryption Notes

- The Go code intentionally avoids third-party Go dependencies for now.
- SQLite cache validation, contact querying, and message querying use the
  system `sqlite3` binary.
- Open decrypted cache DBs with an immutable read-only URI:
  `file://...?mode=ro&immutable=1`
  This avoids SQLite `unable to open database file (14)` issues with read-only
  mode and WAL/SHM side files.
- SQLCipher 4 raw key validation uses page 1 HMAC. Do not overwrite the previous
  cache on decrypt or validation failure.

## Verification

For code changes, run:

```sh
GOCACHE=$(pwd)/.cache/go-build go test ./...
GOCACHE=$(pwd)/.cache/go-build go vet ./...
```

For help behavior, useful smoke checks are:

```sh
go run ./cmd/weview --help
go run ./cmd/weview init --help
go run ./cmd/weview contacts
go run ./cmd/weview contacts --help
go run ./cmd/weview messages
go run ./cmd/weview messages --help
go run ./cmd/weview timeline
go run ./cmd/weview timeline --help
```
