# Weview

Weview is a local-first CLI for reading macOS WeChat data from the user's own machine.

V1 is a macOS WeChat 4.x contact database reader written in Go.

Scope:

- Auto-detect `~/Library/Containers/com.tencent.xinWeChat/Data/Documents/xwechat_files/<account>/db_storage/contact/contact.db`
- Auto-get the SQLCipher raw DB key from the running WeChat process
- Decrypt only the main `contact/contact.db` into `~/.weview/cache/<account>/contact/contact.db`
- Serve a local daemon over `~/.weview/weview.sock`
- List all contacts from CLI

Out of scope for V1:

- WAL patching
- Public Web API
- Message, media, image, voice, or transfer export
- Windows/Linux and macOS WeChat 3.x

## Commands

```sh
go run ./cmd/weview init
go run ./cmd/weview daemon
go run ./cmd/weview contacts --format table
go run ./cmd/weview contacts --format json
go run ./cmd/weview contacts --format jsonl
go run ./cmd/weview contacts --refresh
go run ./cmd/weview contacts --kind friend
go run ./cmd/weview contacts --kind chatroom
go run ./cmd/weview contacts --help
```

`weview contacts` uses the daemon when it is running. If the daemon is not running, it performs a local ensure-key, decrypt, and query pass.

Run `weview init` at the beginning. In normal use it only needs to be run once;
it saves the verified contact DB key to `~/.weview/keys.json`.

Contact `kind` values:

- `friend`: ordinary private-chat contacts, currently `local_type = 1`, excluding `@chatroom` and `gh_*`
- `chatroom`: group chats, detected by usernames ending in `@chatroom`
- `other`: official accounts, enterprise contacts, non-friend room members, and special/system contacts
- `all`: no filtering

## Local State

- Config directory: `~/.weview/`
- Key store: `~/.weview/keys.json`, mode `0600`
- Contact cache: `~/.weview/cache/<account>/contact/contact.db`
- Daemon socket: `~/.weview/weview.sock`

The CLI prints only the key fingerprint, never the full DB key.

If an older run created root-owned state, fix it once:

```sh
sudo chown -R "$USER":staff ~/.weview
```

## Permissions

Key scanning reads WeChat process memory. If scanning fails, make sure WeChat is running and run with the required macOS permission, for example `sudo`, or a WeChat binary signed with `get-task-allow` for local research.

The contact query and decrypted-cache validation use the system `sqlite3` executable.
