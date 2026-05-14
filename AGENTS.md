# Agent Notes for Weview

Weview is a local-first CLI for reading data from the user's own macOS WeChat.
The V1 implementation is intentionally narrow: initialize the contact DB key,
decrypt `contact/contact.db`, run a local daemon, and list contacts/groups from
the decrypted contact cache.

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
  reuses the contact DB key, verifies it against page 1 HMAC, and saves it in
  `~/.weview/keys.json`. Normal use should run it once at the beginning.
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
  service. It should focus on key/cache setup, refresh, source DB watching,
  `health`, `refresh_contacts`, and `stop`.
- The only supported daemon CLI forms are `weview daemon`, `weview daemon start`,
  `weview daemon stop`, and `weview daemon status`. Bare `weview daemon` must
  show the same help as `weview daemon --help` and must not start the daemon.
- `weview contacts ...` should always read contacts directly from the local
  decrypted cache, then apply filtering, sorting, pagination, counts, and output
  formatting in the CLI path.
- If `contacts --refresh` is used and the daemon is running, the CLI may ask the
  daemon to refresh first, then still read the cache directly. If the daemon is
  not running, the CLI may refresh the cache in-process.
- Do not reintroduce daemon-side `list_contacts` unless the product direction
  explicitly changes to a real query service, Web API, or MCP service.
- The daemon uses the internal Unix socket `~/.weview/weview.sock`. Treat this as
  internal transport, not a public Web API.
- Decrypted contact cache path is:
  `~/.weview/cache/<account>/contact/contact.db`
- Use readable account directory names, not base64, unless a future account name
  contains unsafe path characters. Unsafe characters should be replaced with `_`.
- When running under `sudo`, state written under `~/.weview` should be chowned
  back to the original user from `SUDO_UID`/`SUDO_GID`.

## SQLite and Decryption Notes

- The Go code intentionally avoids third-party Go dependencies for now.
- SQLite cache validation and contact querying use the system `sqlite3` binary.
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
```
