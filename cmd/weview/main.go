package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"weview/internal/app"
	"weview/internal/contacts"
	"weview/internal/daemon"
	"weview/internal/key"
	"weview/internal/media"
	"weview/internal/messages"
	"weview/internal/timeline"
)

const messageEnvelopeSchemaVersion = 1

func main() {
	log.SetFlags(log.LstdFlags)
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer, stderr io.Writer) error {
	if len(args) == 0 {
		usage(stdout)
		return nil
	}
	ctx := context.Background()
	switch args[0] {
	case "init":
		return runInit(ctx, args[1:], stdout)
	case "daemon":
		return runDaemon(args[1:], stdout)
	case "contact", "contacts":
		return runContacts(ctx, args[1:], stdout)
	case "messages":
		return runMessages(ctx, args[1:], stdout)
	case "timeline":
		return runTimeline(ctx, args[1:], stdout)
	case "contract":
		if len(args) > 1 && hasHelp(args[1:]) {
			contactsUsage(stdout)
			fmt.Fprintln(stdout, "\nNote: the official command is `contacts`; `contract --help` is accepted only as a typo-friendly help alias.")
			return nil
		}
		usage(stderr)
		return fmt.Errorf("unknown command: contract (did you mean contacts?)")
	case "help", "-h", "--help":
		if len(args) > 1 {
			return commandHelp(args[1], stdout, stderr)
		}
		usage(stdout)
		return nil
	default:
		usage(stderr)
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, `weview - Read local WeChat data

Weview is a local-first CLI for reading macOS WeChat 4.x data from the
user's own machine. It can obtain database keys, decrypt local WeChat
databases into ~/.weview/cache, list contacts or contact-table groups, and
query message history for an explicit username.

Commands:
  weview init       First-time setup: detect WeChat, get supported DB keys,
                    and save them locally. Usually run once at the beginning.
  weview daemon     Show daemon help.
  weview contacts   List contacts from the decrypted contact cache.
  weview messages   List messages for an explicit username.
  weview timeline   List messages across selected conversations by time.
  weview help CMD   Show detailed help for a command.

Common examples:
  sudo weview init
  weview contacts --format json
  weview contacts --kind friend --format jsonl
  weview contacts --kind friend --format csv
  weview contacts --kind friend --query AI --limit 20 --format json
  weview contacts --kind chatroom --format table
  weview contacts --refresh --format json
  weview messages --username wxid_xxx --start "2026-05-01" --end "2026-05-14" --format json
  weview messages --username wxid_xxx --date today --limit 100 --format json
  weview messages --username wxid_xxx --after-seq 1773421286000 --limit 100 --format jsonl
  weview timeline --kind chatroom --query AI --date today --limit 200 --format json
  weview daemon start
  weview daemon status

Machine-readable usage:
  Use --format json for machine-readable output. messages and timeline use a
  JSON envelope with meta and items; contacts uses a JSON array.
  Use --format jsonl for one JSON object per line.
  Use --format csv for spreadsheet/shell pipelines.
  Use --kind friend for ordinary private-chat contacts.
  Use --kind chatroom for groups present in the contact table.

Current scope:
  Supported: macOS WeChat 4.x contact/contact.db and message/message_*.db
             history reads by explicit username, including local image/video
             path resolution when media files are available.
  Not included yet: WAL patching, public Web API.

Run:
  weview init --help
  weview contacts --help
  weview messages --help
  weview timeline --help
  weview daemon --help`)
}

func commandHelp(command string, stdout io.Writer, stderr io.Writer) error {
	switch command {
	case "init":
		initUsage(stdout)
	case "daemon":
		daemonUsage(stdout)
	case "contact", "contacts":
		contactsUsage(stdout)
	case "messages":
		messagesUsage(stdout)
	case "timeline":
		timelineUsage(stdout)
	default:
		usage(stderr)
		return fmt.Errorf("unknown command: %s", command)
	}
	return nil
}

func initUsage(w io.Writer) {
	fmt.Fprintln(w, `weview init - First-time setup for reading local WeChat data

Usage:
  sudo weview init [--verbose]
  sudo go run ./cmd/weview init [--verbose]

When to run:
  Run this at the beginning before using contacts/daemon.
  In normal use, run it once. Run it again only if WeChat changes account,
  contact/contact.db salt changes, the saved key becomes invalid, or WeChat is
  reinstalled/updated in a way that changes the database key.

What it does:
  1. Detects the current macOS WeChat 4.x account under:
     ~/Library/Containers/com.tencent.xinWeChat/Data/Documents/xwechat_files/<account>/db_storage
  2. Finds required databases:
     contact/contact.db
     message/message_[number].db
     and best-effort auxiliary message databases:
     message/message_fts.db, message/message_resource.db, message/message_revoke.db
  3. Reads each discovered DB page 1 salt.
  4. Reuses existing valid keys from ~/.weview/cache/<account>/keys.json, or
     scans the running WeChat process memory for missing SQLCipher raw keys.
  5. Verifies each key with page 1 HMAC.
  6. Saves version 1 account metadata and keys in
     ~/.weview/cache/<account>/keys.json with mode 0600.

Required DB key failures stop init. Auxiliary message DB failures are reported
as warnings and can be retried later with sudo weview init.

Output fields:
  account       WeChat account directory name.
  data_dir      Source db_storage directory.
  keys_total    Number of DB keys prepared.
  keys_scanned  Number of keys found in this run.
  keys_reused   Number of saved keys reused.
  warnings      Optional DBs that could not be prepared in this run.

Flags:
  --verbose     Also print each DB path, short key fingerprint, and status.

Notes:
  The full key is never printed, even with --verbose.
  Key scanning needs WeChat running and macOS permission to read its process
  memory. On Hardened Runtime WeChat builds, sudo alone may not be enough; use a
  local GUI terminal with Developer Tools permission or ad-hoc re-sign WeChat.`)
}

const (
	daemonForegroundEnv   = "WEVIEW_DAEMON_FOREGROUND"
	daemonSupportedForms  = "`weview daemon`, `weview daemon start`, `weview daemon stop`, `weview daemon status`"
	daemonStartWait       = 15 * time.Second
	daemonStopWait        = 5 * time.Second
	daemonStatusPollEvery = 100 * time.Millisecond
)

func daemonUsage(w io.Writer) {
	fmt.Fprintln(w, `weview daemon - Manage the local WeChat contact cache daemon

Usage:
  weview daemon
  weview daemon start
  weview daemon stop
  weview daemon status

Supported forms:
  weview daemon         Show this help.
  weview daemon start   Start the daemon in the background.
  weview daemon stop    Stop the background daemon.
  weview daemon status  Check whether ~/.weview/weview.sock responds to health.

Flags:
  No daemon flags are currently supported except -h/--help/help.

What it does:
  1. Uses keys prepared by weview init.
  2. Decrypts contact/contact.db into:
     ~/.weview/cache/<account>/contact/contact.db
  3. Decrypts supported message DBs into:
     ~/.weview/cache/<account>/message/
  4. Opens an internal Unix socket:
     ~/.weview/weview.sock
  5. Watches contact and supported message DB files and refreshes affected
     caches after a debounce delay when they change.
     The current account is resolved from DB files opened by WeChat, so account
     switches while the daemon is running move to the new account cache.
     Unchanged DBs are skipped using ~/.weview/cache/<account>/mtime.json.

Internal daemon actions:
  health
  refresh_contacts
  refresh_messages
  stop

Notes:
  This is an internal local transport, not a public Web API.
  daemon start writes daemon logs to ~/.weview/weview.log.
  V1 does not patch or stream .db-wal, so refresh is near-real-time after WeChat
  checkpoints/writes the main DB.`)
}

func contactsUsage(w io.Writer) {
	fmt.Fprintln(w, `weview contacts - List WeChat contacts and contact-table groups

Usage:
  weview contacts --format table|json|jsonl|csv [flags]
  weview contacts --count [flags]
  weview contacts --help

Alias:
  weview contact

No-argument behavior:
  weview contacts is intentionally the same as weview contacts --help.
  To query data, pass an explicit output/filter flag such as --format or --kind.

Flags:
  --format table   Human-readable table output.
  --format json    Machine-readable JSON array.
  --format jsonl   Machine-readable newline-delimited JSON, one contact per line.
  --format csv     Machine-readable CSV with header row.

  --kind all       Return every row selected from the contact table.
  --kind friend    Ordinary private-chat contacts.
  --kind chatroom  Group chats.
  --kind other     Official accounts, enterprise contacts, non-friend room members,
                   and special/system contacts.

  --query TEXT     Case-insensitive contains search over username, alias,
                   remark, and nick_name.
  --username TEXT  Exact username lookup, e.g. wxid_* or *@chatroom.
  --limit N        Return at most N rows. 0 means no limit.
  --offset N       Skip N rows before returning results. Requires stable sorting
                   for paging; default sort is username.
  --sort username  Sort by username. This is the default and best for paging.
  --sort name      Sort by display name: remark, then nick_name, then alias,
                   then username.
  --count          Output only the number of rows after filters. Pagination
                   flags --limit and --offset are ignored for counts.

  --refresh        Before listing, decrypt the source contact/contact.db into the
                   local cache again. Without --refresh, uses the existing cache
                   when available.

Output fields:
  username    Stable WeChat username, e.g. wxid_* or *@chatroom.
  alias       WeChat ID / alias when present.
  remark      Your remark for the contact.
  nick_name   Contact nickname from WeChat.
  head_url    Avatar image URL.
  kind        friend, chatroom, or other.

Examples for humans:
  weview contacts --format table
  weview contacts --kind friend --format table
  weview contacts --kind chatroom --format table
  weview contacts --kind friend --query AI --limit 20 --format table

Examples for AI/tools:
  weview contacts --format json
  weview contacts --kind friend --format json
  weview contacts --kind chatroom --format jsonl
  weview contacts --kind friend --format csv
  weview contacts --kind friend --query AI --limit 20 --offset 0 --sort username --format json
  weview contacts --username wxid_xxx --format json
  weview contacts --kind friend --count
  weview contacts --refresh --format json

Runtime behavior:
  This command always reads contacts from the local decrypted cache.
  If --refresh is used and the daemon is running, it asks the daemon to refresh
  the cache first; otherwise it refreshes the cache in this process.`)
}

func messagesUsage(w io.Writer) {
	fmt.Fprintln(w, `weview messages - List WeChat messages for one username

Usage:
  weview messages --username USERNAME --format table|json|jsonl|csv [flags]
  weview messages --help

Required:
  --username TEXT  Exact WeChat username, e.g. wxid_* or *@chatroom.

Flags:
  --format table   Human-readable table output.
  --format json    JSON envelope with meta and items.
  --format jsonl   Machine-readable newline-delimited JSON, one message per line.
  --format csv     Machine-readable CSV with header row.

  --start TIME     Inclusive start time. Supports Unix seconds, YYYY-MM-DD,
                   YYYY-MM-DD HH:MM, YYYY-MM-DD HH:MM:SS, or RFC3339.
  --end TIME       Inclusive end time. Date-only values include the full day.
  --date today|yesterday|YYYY-MM-DD
                   Select one full local day. Cannot be combined with --start
                   or --end.
  --after-seq N    Return rows with seq greater than N. Use this for cursor-style
                   "next page after this message" reads.
  --limit N        Return at most N rows after global time sorting. 0 means no limit.
  --offset N       Skip N rows after global time sorting.
  --source         Include source DB/table/local row metadata for debugging.
  --refresh        Decrypt message text shards and supported message-related
                   DBs into the local cache before querying. Without --refresh,
                   uses the existing complete cache when available; otherwise
                   refreshes in this process.

Output fields:
  id                Stable local message id for this cache row.
  chat_username     Conversation username requested by --username.
  chat_kind         friend, chatroom, other, or unknown.
  chat_display_name Conversation display name: remark, nick_name, alias, then
                    username.
  chat_alias        Conversation alias when present.
  chat_remark       Your remark for the conversation when present.
  chat_nick_name    Conversation nickname when present.
  from_username     Actual sender username when known. "self" is only used as a
                    fallback when WeChat does not expose the local username.
  direction         out, in, or unknown.
  is_self           Whether this row is from the local user.
  is_chatroom       Whether chat_username is a group chat.
  seq               WeChat sort sequence; pass it to --after-seq for cursor reads.
  server_id         WeChat server message id when present.
  create_time       Unix timestamp from WeChat.
  time              Local formatted time.
  type              Lower 32 bits of WeChat local_type.
  sub_type          Upper 32 bits of WeChat local_type.
  content           Raw decoded message content. XML stays XML.
  content_detail    Parsed convenience fields such as type, text, title, url,
                    image/video metadata, and local media paths when available.
  content_encoding  text, zstd, zstd_error, or invalid_hex.

Source fields:
  Only shown when --source is passed. They are useful for debugging cache/shard
  behavior, but are not needed for normal chat-history reads.

JSON output:
  --format json returns {"meta": ..., "items": [...]}.
  meta.schema_version identifies the envelope contract, and meta.timezone shows
  the local timezone used to interpret --date, --start, --end, and date-only
  bounds.
  meta.next_after_seq and meta.next_args are included when another page exists.
  Reliable clients should continue with meta.next_args rather than --offset.
  jsonl, csv, and table outputs remain item-only.

Examples:
  weview messages --username wxid_xxx --format table
  weview messages --username wxid_xxx --date today --limit 100 --format json
  weview messages --username wxid_xxx --start "2026-05-01" --end "2026-05-14" --format json
  weview messages --username wxid_xxx --after-seq 1773421286000 --limit 100 --format jsonl
  weview messages --username 123@chatroom --limit 100 --offset 0 --format jsonl
  weview messages --username wxid_xxx --source --refresh --format json

Runtime behavior:
  This command reads message rows from local decrypted message caches and merges
  all message/message_*.db shards before applying pagination. Results are sorted
  by create_time ascending, then seq, source local id, and source shard.
  V1 does not use a message index for this command; broad ranges in large chats
  can be slow. Prefer bounded time windows and --after-seq pagination.
  Image and video messages in the returned page are resolved to local files
  automatically. WeChat .dat images and recognizable .dat videos are decoded
  or normalized into ~/.weview/cache/<account>/media/.
  If a required message DB key is missing, run sudo weview init first.`)
}

func timelineUsage(w io.Writer) {
	fmt.Fprintln(w, `weview timeline - List WeChat messages across selected conversations

Usage:
  weview timeline --kind all|friend|chatroom|other --query TEXT --date today --format json [flags]
  weview timeline --username USERNAME --start TIME --end TIME --format json [flags]
  weview timeline --help

Conversation selection:
  --kind all       Select from all conversation rows in the contact cache.
  --kind friend    Select ordinary private-chat contacts.
  --kind chatroom  Select group chats.
  --kind other     Select official accounts, enterprise contacts, and other rows.
  --query TEXT     Case-insensitive conversation filter over username, alias,
                   remark, and nick_name. This does not search message content.
  --username TEXT  Exact conversation username. Cannot be combined with --kind
                   or --query.

Time range:
  --date today|yesterday|YYYY-MM-DD
                   Select one full local day. Cannot be combined with --start
                   or --end.
  --start TIME     Inclusive start time. Required with --end when --date is not used.
  --end TIME       Inclusive end time. Required with --start when --date is not used.

Pagination:
  --limit N        Return at most N rows. Default 200. Maximum 1000.
  --cursor TOKEN   Continue from meta.next_cursor. Keep the same query arguments.

Other flags:
  --format table   Human-readable table output.
  --format json    JSON envelope with meta and items. Use this for reliable paging.
  --format jsonl   Newline-delimited JSON, one message item per line.
  --format csv     CSV message items with header row.
  --source         Include source DB/table/local row metadata for debugging.
  --refresh        Refresh contact and message caches before querying.

Examples:
  weview timeline --kind chatroom --query AI --date today --limit 200 --format json
  weview timeline --kind chatroom --query AI --start "2026-05-14" --end "2026-05-14" --format json
  weview timeline --kind chatroom --query AI --start "2026-05-14 00:00:00" --end "2026-05-14 23:59:59" --limit 200 --cursor TOKEN --format json
  weview timeline --username wxid_xxx --date yesterday --format jsonl

Runtime behavior:
  This command selects conversations from the local contact cache, reads
  matching message rows from local decrypted message caches, merges them into a
  single create_time ascending timeline, and applies cursor pagination globally.
  V1 does not maintain a message index. Wide ranges or broad selectors can be
  very slow because the command fans out across conversations and message
  shards before applying the global limit. Prefer small time windows and narrow
  selectors such as --kind chatroom --query AI.
  JSON output includes meta.schema_version and meta.timezone. It also includes
  meta.next_args so AI/tool callers can continue paging without understanding
  the cursor internals.`)
}

func hasHelp(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "help" {
			return true
		}
	}
	return false
}

func runInit(ctx context.Context, args []string, stdout io.Writer) error {
	if hasHelp(args) {
		initUsage(stdout)
		return nil
	}
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	verbose := fs.Bool("verbose", false, "print per-database key fingerprints and statuses")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected init argument: %s", fs.Arg(0))
	}
	ensureResult, err := key.EnsureSupportedKeys(ctx)
	if err != nil {
		return err
	}
	results := ensureResult.Keys
	if len(results) == 0 {
		return fmt.Errorf("no supported database found")
	}
	return writeInitOutput(stdout, results, ensureResult.Warnings, *verbose)
}

func writeInitOutput(stdout io.Writer, results []key.EnsureResult, warnings []key.EnsureWarning, verbose bool) error {
	fmt.Fprintf(stdout, "account: %s\n", results[0].Target.Account)
	fmt.Fprintf(stdout, "data_dir: %s\n", results[0].Target.DataDir)
	fmt.Fprintf(stdout, "keys_total: %d\n", len(results))
	scanned := 0
	reused := 0
	for _, res := range results {
		if res.Reused {
			reused++
		} else {
			scanned++
		}
	}
	fmt.Fprintf(stdout, "keys_scanned: %d\n", scanned)
	fmt.Fprintf(stdout, "keys_reused: %d\n", reused)
	if verbose {
		fmt.Fprintln(stdout, "keys:")
		for _, res := range results {
			status := "scanned"
			if res.Reused {
				status = "reused"
			}
			fmt.Fprintf(stdout, "  %s fingerprint=%s status=%s\n", res.Target.DBRelPath, res.Entry.Fingerprint, status)
		}
	}
	if len(warnings) > 0 {
		fmt.Fprintf(stdout, "warnings_total: %d\n", len(warnings))
		fmt.Fprintln(stdout, "warnings:")
		for _, warning := range warnings {
			fmt.Fprintf(stdout, "  %s %s\n", warning.DBRelPath, warning.Message)
		}
	}
	return nil
}

func runDaemon(args []string, stdout io.Writer) error {
	if os.Getenv(daemonForegroundEnv) == "1" && len(args) == 0 {
		return runDaemonForeground(stdout)
	}
	if len(args) == 0 || hasHelp(args) {
		daemonUsage(stdout)
		return nil
	}
	switch args[0] {
	case "start":
		return runDaemonStart(args[1:], stdout)
	case "stop":
		return runDaemonStop(args[1:], stdout)
	case "status":
		return runDaemonStatus(args[1:], stdout)
	default:
		return fmt.Errorf("unknown daemon command: %s; supported forms are %s", args[0], daemonSupportedForms)
	}
}

func runDaemonForeground(stdout io.Writer) error {
	socketPath, err := app.SocketPath()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Fprintf(stdout, "daemon socket: %s\n", socketPath)
	fmt.Fprintln(stdout, "initializing local caches...")
	server := &daemon.Server{SocketPath: socketPath, Shutdown: stop}
	if err := server.Run(ctx); err != nil {
		return err
	}
	return nil
}

func runDaemonStart(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("daemon start", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected daemon start argument: %s", fs.Arg(0))
	}
	socketPath, err := app.SocketPath()
	if err != nil {
		return err
	}
	client := daemon.Client{SocketPath: socketPath, Timeout: 500 * time.Millisecond}
	fmt.Fprintf(stdout, "daemon socket: %s\n", socketPath)
	if client.Healthy(context.Background()) {
		fmt.Fprintln(stdout, "status: already running")
		return nil
	}

	logPath, err := app.LogPath()
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	if err := app.ChownForSudo(logPath); err != nil {
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "daemon")
	cmd.Env = append(os.Environ(), daemonForegroundEnv+"=1")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "log: %s\n", logPath)
	fmt.Fprintf(stdout, "pid: %d\n", cmd.Process.Pid)

	exitCh := make(chan error, 1)
	go func() {
		exitCh <- cmd.Wait()
	}()
	if ok, err := waitForDaemonHealthy(context.Background(), client, daemonStartWait, exitCh); err != nil {
		return err
	} else if ok {
		fmt.Fprintln(stdout, "status: running")
		return nil
	}
	fmt.Fprintln(stdout, "status: starting")
	fmt.Fprintln(stdout, "note: daemon has not responded to health yet; check status or the log file.")
	return nil
}

func runDaemonStop(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("daemon stop", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected daemon stop argument: %s", fs.Arg(0))
	}
	socketPath, err := app.SocketPath()
	if err != nil {
		return err
	}
	client := daemon.Client{SocketPath: socketPath, Timeout: 500 * time.Millisecond}
	fmt.Fprintf(stdout, "daemon socket: %s\n", socketPath)
	if !client.Healthy(context.Background()) {
		fmt.Fprintln(stdout, "status: stopped")
		return nil
	}
	if _, err := client.Call(context.Background(), daemon.ActionStop); err != nil {
		return err
	}
	if waitForDaemonStopped(context.Background(), client, daemonStopWait) {
		fmt.Fprintln(stdout, "status: stopped")
		return nil
	}
	fmt.Fprintln(stdout, "status: stopping")
	return nil
}

func runDaemonStatus(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("daemon status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected daemon status argument: %s", fs.Arg(0))
	}
	socketPath, err := app.SocketPath()
	if err != nil {
		return err
	}
	client := daemon.Client{SocketPath: socketPath, Timeout: 500 * time.Millisecond}
	fmt.Fprintf(stdout, "daemon socket: %s\n", socketPath)
	if client.Healthy(context.Background()) {
		fmt.Fprintln(stdout, "status: running")
		return nil
	}
	fmt.Fprintln(stdout, "status: stopped")
	return nil
}

func waitForDaemonHealthy(ctx context.Context, client daemon.Client, timeout time.Duration, exitCh <-chan error) (bool, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(daemonStatusPollEvery)
	defer ticker.Stop()
	for {
		if client.Healthy(ctx) {
			return true, nil
		}
		select {
		case err := <-exitCh:
			if err != nil {
				return false, fmt.Errorf("daemon exited before becoming healthy: %w", err)
			}
			return false, fmt.Errorf("daemon exited before becoming healthy")
		case <-deadline.C:
			return false, nil
		case <-ticker.C:
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
}

func waitForDaemonStopped(ctx context.Context, client daemon.Client, timeout time.Duration) bool {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(daemonStatusPollEvery)
	defer ticker.Stop()
	for {
		if !client.Healthy(ctx) {
			return true
		}
		select {
		case <-deadline.C:
			return false
		case <-ticker.C:
		case <-ctx.Done():
			return false
		}
	}
}

func runContacts(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 || hasHelp(args) {
		contactsUsage(stdout)
		return nil
	}
	fs := flag.NewFlagSet("contacts", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	format := fs.String("format", "table", "table, json, jsonl, or csv")
	kind := fs.String("kind", contacts.KindAll, "all, friend, chatroom, or other")
	query := fs.String("query", "", "contains search over username, alias, remark, and nick_name")
	username := fs.String("username", "", "exact username lookup")
	sortBy := fs.String("sort", "username", "username or name")
	limit := fs.Int("limit", 0, "maximum rows to return; 0 means no limit")
	offset := fs.Int("offset", 0, "rows to skip before returning results")
	countOnly := fs.Bool("count", false, "output only count after filters")
	refresh := fs.Bool("refresh", false, "refresh decrypted contact cache before listing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !validFormat(*format) {
		return fmt.Errorf("invalid format %q: use table, json, jsonl, or csv", *format)
	}
	if !validKind(*kind) {
		return fmt.Errorf("invalid kind %q: use all, friend, chatroom, or other", *kind)
	}
	if !validSort(*sortBy) {
		return fmt.Errorf("invalid sort %q: use username or name", *sortBy)
	}
	if *limit < 0 {
		return fmt.Errorf("invalid limit %d: must be >= 0", *limit)
	}
	if *offset < 0 {
		return fmt.Errorf("invalid offset %d: must be >= 0", *offset)
	}

	list, err := listContacts(ctx, *refresh)
	if err != nil {
		return err
	}
	opts := contacts.QueryOptions{
		Kind:     *kind,
		Query:    *query,
		Username: *username,
		Sort:     *sortBy,
		Limit:    *limit,
		Offset:   *offset,
	}
	if *countOnly {
		opts.Limit = 0
		opts.Offset = 0
		list = contacts.ApplyQueryOptions(list, opts)
		return writeCount(stdout, len(list), *format)
	}
	list = contacts.ApplyQueryOptions(list, opts)
	return writeContacts(stdout, list, *format)
}

func runMessages(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 || hasHelp(args) {
		messagesUsage(stdout)
		return nil
	}
	fs := flag.NewFlagSet("messages", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	format := fs.String("format", "table", "table, json, jsonl, or csv")
	username := fs.String("username", "", "exact WeChat username")
	dateText := fs.String("date", "", "today, yesterday, or YYYY-MM-DD")
	startText := fs.String("start", "", "inclusive start time")
	endText := fs.String("end", "", "inclusive end time")
	afterSeq := fs.Int64("after-seq", 0, "return rows with seq greater than this value")
	limit := fs.Int("limit", 0, "maximum rows to return after sorting; 0 means no limit")
	offset := fs.Int("offset", 0, "rows to skip after sorting")
	includeSource := fs.Bool("source", false, "include source DB/table/local row metadata")
	refresh := fs.Bool("refresh", false, "refresh decrypted message caches before listing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !validFormat(*format) {
		return fmt.Errorf("invalid format %q: use table, json, jsonl, or csv", *format)
	}
	if strings.TrimSpace(*username) == "" {
		return fmt.Errorf("--username is required")
	}
	if *limit < 0 {
		return fmt.Errorf("invalid limit %d: must be >= 0", *limit)
	}
	if *offset < 0 {
		return fmt.Errorf("invalid offset %d: must be >= 0", *offset)
	}
	if *afterSeq < 0 {
		return fmt.Errorf("invalid after-seq %d: must be >= 0", *afterSeq)
	}
	start, end, hasStart, hasEnd, err := parseMessageRange(*dateText, *startText, *endText)
	if err != nil {
		return err
	}

	queryLimit := *limit
	if *format == "json" && *limit > 0 {
		queryLimit = *limit + 1
	}
	cachePaths, err := messageCachePaths(ctx, *refresh)
	if err != nil {
		return err
	}
	usernameValue := strings.TrimSpace(*username)
	target, err := key.DiscoverContactDB()
	if err != nil {
		return err
	}
	cacheDir, err := media.MediaCacheDir(target.Account)
	if err != nil {
		return err
	}
	resolver := newMediaResolver(target, cacheDir)
	list, err := messages.NewService(cachePaths).List(ctx, messages.QueryOptions{
		Username:      usernameValue,
		Start:         start,
		End:           end,
		AfterSeq:      *afterSeq,
		HasStart:      hasStart,
		HasEnd:        hasEnd,
		HasAfterSeq:   *afterSeq > 0,
		IncludeSource: *includeSource,
		MediaResolver: &resolver,
		Limit:         queryLimit,
		Offset:        *offset,
	})
	if err != nil {
		return err
	}
	messages.ApplyChatInfo(list, loadExistingChatInfoMap(ctx))
	if *format == "json" {
		page, hasMore := trimMessagePage(list, *limit)
		meta := messagesMeta{
			SchemaVersion: messageEnvelopeSchemaVersion,
			Timezone:      localTimezoneName(),
			Mode:          "messages",
			Username:      usernameValue,
			Start:         formatMetaTime(start, hasStart),
			End:           formatMetaTime(end, hasEnd),
			Limit:         *limit,
			Returned:      len(page),
			HasMore:       hasMore,
		}
		if hasMore && len(page) > 0 {
			meta.NextAfterSeq = page[len(page)-1].Seq
			meta.NextArgs = buildMessagesNextArgs(usernameValue, start, hasStart, end, hasEnd, meta.NextAfterSeq, *limit, *includeSource)
		}
		return writeMessageEnvelope(stdout, meta, page)
	}
	return writeMessages(stdout, list, *format, *includeSource)
}

func runTimeline(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 || hasHelp(args) {
		timelineUsage(stdout)
		return nil
	}
	fs := flag.NewFlagSet("timeline", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	format := fs.String("format", "table", "table, json, jsonl, or csv")
	kind := fs.String("kind", contacts.KindAll, "all, friend, chatroom, or other")
	query := fs.String("query", "", "conversation contains search over username, alias, remark, and nick_name")
	username := fs.String("username", "", "exact WeChat username")
	dateText := fs.String("date", "", "today, yesterday, or YYYY-MM-DD")
	startText := fs.String("start", "", "inclusive start time")
	endText := fs.String("end", "", "inclusive end time")
	limit := fs.Int("limit", 200, "maximum rows to return; max 1000")
	cursor := fs.String("cursor", "", "opaque cursor from meta.next_cursor")
	includeSource := fs.Bool("source", false, "include source DB/table/local row metadata")
	refresh := fs.Bool("refresh", false, "refresh contact and message caches before listing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected timeline argument: %s", fs.Arg(0))
	}
	if !validFormat(*format) {
		return fmt.Errorf("invalid format %q: use table, json, jsonl, or csv", *format)
	}
	if !validKind(*kind) {
		return fmt.Errorf("invalid kind %q: use all, friend, chatroom, or other", *kind)
	}
	if *limit <= 0 {
		return fmt.Errorf("invalid limit %d: must be > 0", *limit)
	}
	if *limit > 1000 {
		return fmt.Errorf("invalid limit %d: must be <= 1000", *limit)
	}
	usernameValue := strings.TrimSpace(*username)
	queryValue := strings.TrimSpace(*query)
	kindProvided := flagProvided(args, "kind")
	if usernameValue != "" && (kindProvided || queryValue != "") {
		return fmt.Errorf("--username cannot be combined with --kind or --query")
	}
	if usernameValue == "" && !kindProvided && queryValue == "" {
		return fmt.Errorf("timeline requires --username, --kind, or --query")
	}
	start, end, startLabel, endLabel, err := parseTimelineRange(*dateText, *startText, *endText)
	if err != nil {
		return err
	}

	chats, matchedChats, err := selectTimelineChats(ctx, usernameValue, *kind, queryValue, kindProvided, *refresh)
	if err != nil {
		return err
	}
	cachePaths, err := messageCachePaths(ctx, *refresh)
	if err != nil {
		return err
	}
	target, err := key.DiscoverContactDB()
	if err != nil {
		return err
	}
	cacheDir, err := media.MediaCacheDir(target.Account)
	if err != nil {
		return err
	}
	queryIdentity := timeline.QueryIdentity{
		Username: usernameValue,
		Start:    start,
		End:      end,
	}
	if usernameValue == "" {
		queryIdentity.Kind = *kind
		queryIdentity.Query = queryValue
	}
	queryHash, err := timeline.QueryHash(queryIdentity)
	if err != nil {
		return err
	}
	result, err := timeline.List(ctx, messages.NewService(cachePaths), timeline.QueryOptions{
		Chats:         chats,
		Start:         start,
		End:           end,
		Limit:         *limit,
		Cursor:        *cursor,
		QueryHash:     queryHash,
		IncludeSource: *includeSource,
	})
	if err != nil {
		return err
	}
	resolver := newMediaResolver(target, cacheDir)
	messages.EnrichMediaDetails(result.Items, &resolver)
	if *format == "json" {
		meta := timelineMeta{
			SchemaVersion: messageEnvelopeSchemaVersion,
			Timezone:      localTimezoneName(),
			Mode:          "timeline",
			Kind:          "",
			Query:         "",
			Username:      usernameValue,
			Start:         startLabel,
			End:           endLabel,
			Limit:         *limit,
			Returned:      len(result.Items),
			MatchedChats:  matchedChats,
			HasMore:       result.HasMore,
		}
		if usernameValue == "" {
			meta.Kind = *kind
			meta.Query = queryValue
		}
		if result.HasMore {
			meta.NextCursor = result.NextCursor
			meta.NextArgs = buildTimelineNextArgs(usernameValue, *kind, queryValue, startLabel, endLabel, *limit, result.NextCursor, *includeSource)
		}
		return writeMessageEnvelope(stdout, meta, result.Items)
	}
	return writeMessages(stdout, result.Items, *format, *includeSource)
}

type messagesMeta struct {
	SchemaVersion int      `json:"schema_version"`
	Timezone      string   `json:"timezone"`
	Mode          string   `json:"mode"`
	Username      string   `json:"username"`
	Start         string   `json:"start"`
	End           string   `json:"end"`
	Limit         int      `json:"limit"`
	Returned      int      `json:"returned"`
	HasMore       bool     `json:"has_more"`
	NextAfterSeq  int64    `json:"next_after_seq,omitempty"`
	NextArgs      []string `json:"next_args,omitempty"`
}

type timelineMeta struct {
	SchemaVersion int      `json:"schema_version"`
	Timezone      string   `json:"timezone"`
	Mode          string   `json:"mode"`
	Kind          string   `json:"kind,omitempty"`
	Query         string   `json:"query,omitempty"`
	Username      string   `json:"username,omitempty"`
	Start         string   `json:"start"`
	End           string   `json:"end"`
	Limit         int      `json:"limit"`
	Returned      int      `json:"returned"`
	MatchedChats  int      `json:"matched_chats"`
	HasMore       bool     `json:"has_more"`
	NextCursor    string   `json:"next_cursor,omitempty"`
	NextArgs      []string `json:"next_args,omitempty"`
}

type messageEnvelope struct {
	Meta  any                `json:"meta"`
	Items []messages.Message `json:"items"`
}

func selectTimelineChats(ctx context.Context, username string, kind string, query string, kindProvided bool, refresh bool) ([]messages.ChatInfo, int, error) {
	if username != "" {
		if refresh {
			if err := refreshContactCache(ctx); err != nil {
				return nil, 0, err
			}
		}
		info := loadExistingChatInfoMap(ctx)[username]
		if strings.TrimSpace(info.Username) == "" {
			info = messages.ChatInfo{Username: username, Kind: messages.ChatKindUnknown, DisplayName: username}
		}
		return []messages.ChatInfo{info}, 1, nil
	}
	list, err := listContacts(ctx, refresh)
	if err != nil {
		return nil, 0, err
	}
	opts := contacts.QueryOptions{
		Kind:  kind,
		Query: query,
		Sort:  "username",
	}
	if !kindProvided && query != "" {
		opts.Kind = contacts.KindAll
	}
	selected := contacts.ApplyQueryOptions(list, opts)
	chats := make([]messages.ChatInfo, 0, len(selected))
	for _, contact := range selected {
		chats = append(chats, chatInfoFromContact(contact))
	}
	return chats, len(chats), nil
}

func loadExistingChatInfoMap(ctx context.Context) map[string]messages.ChatInfo {
	_, path, ok := key.HasContactCache()
	if !ok {
		return nil
	}
	list, err := contacts.NewService(path).List(ctx)
	if err != nil {
		return nil
	}
	return chatInfoMapFromContacts(list)
}

func chatInfoMapFromContacts(list []contacts.Contact) map[string]messages.ChatInfo {
	out := make(map[string]messages.ChatInfo, len(list))
	for _, contact := range list {
		out[contact.Username] = chatInfoFromContact(contact)
	}
	return out
}

func chatInfoFromContact(contact contacts.Contact) messages.ChatInfo {
	return messages.ChatInfo{
		Username:    contact.Username,
		Kind:        contact.Kind,
		DisplayName: contacts.DisplayName(contact),
		Alias:       contact.Alias,
		Remark:      contact.Remark,
		NickName:    contact.NickName,
	}
}

func listContacts(ctx context.Context, refresh bool) ([]contacts.Contact, error) {
	cachePath, err := contactCachePath(ctx, refresh)
	if err != nil {
		return nil, err
	}
	return contacts.NewService(cachePath).List(ctx)
}

func contactCachePath(ctx context.Context, refresh bool) (string, error) {
	if refresh {
		if err := refreshContactCache(ctx); err != nil {
			return "", err
		}
		if _, path, ok := key.HasContactCache(); ok {
			return path, nil
		}
		return "", fmt.Errorf("contact cache was not found after refresh")
	}

	if _, path, ok := key.HasContactCache(); ok {
		return path, nil
	}
	_, path, err := key.EnsureContactCache(ctx)
	if err != nil {
		return "", err
	}
	return path, nil
}

func refreshContactCache(ctx context.Context) error {
	socketPath, err := app.SocketPath()
	if err != nil {
		return err
	}
	client := daemon.Client{SocketPath: socketPath, Timeout: 5 * time.Second}
	if client.Healthy(ctx) {
		_, err := client.Call(ctx, daemon.ActionRefreshContacts)
		if err != nil {
			return err
		}
		return nil
	}
	_, _, err = key.EnsureContactCache(ctx)
	return err
}

func messageCachePaths(ctx context.Context, refresh bool) ([]string, error) {
	if refresh {
		if err := refreshMessageCaches(ctx); err != nil {
			return nil, err
		}
		paths, allExist, err := key.MessageCachePaths()
		if err != nil {
			return nil, err
		}
		if allExist {
			return paths, nil
		}
		return nil, fmt.Errorf("message cache was not found after refresh")
	}
	paths, allExist, err := key.MessageCachePaths()
	if err != nil {
		return nil, err
	}
	if allExist {
		return paths, nil
	}
	return key.EnsureMessageCaches(ctx)
}

func refreshMessageCaches(ctx context.Context) error {
	socketPath, err := app.SocketPath()
	if err != nil {
		return err
	}
	client := daemon.Client{SocketPath: socketPath, Timeout: time.Minute}
	if client.Healthy(ctx) {
		_, err := client.Call(ctx, daemon.ActionRefreshMessages)
		return err
	}
	_, err = key.EnsureMessageCaches(ctx)
	return err
}

func newMediaResolver(target key.TargetDB, cacheDir string) media.Resolver {
	resourceDBs := []string{}
	if path, err := key.CachePath(target.Account, "message/message_resource.db"); err == nil {
		resourceDBs = append(resourceDBs, path)
	}
	return media.NewResolver(target.DataDir, cacheDir, resourceDBs...)
}

func parseMessageRange(dateText string, startText string, endText string) (int64, int64, bool, bool, error) {
	dateText = strings.TrimSpace(dateText)
	startText = strings.TrimSpace(startText)
	endText = strings.TrimSpace(endText)
	if dateText != "" && (startText != "" || endText != "") {
		return 0, 0, false, false, fmt.Errorf("--date cannot be combined with --start or --end")
	}
	if dateText != "" {
		start, end, err := parseDateRange(dateText)
		if err != nil {
			return 0, 0, false, false, err
		}
		return start, end, true, true, nil
	}
	start, hasStart, err := parseTimeBound(startText, "start", false)
	if err != nil {
		return 0, 0, false, false, err
	}
	end, hasEnd, err := parseTimeBound(endText, "end", true)
	if err != nil {
		return 0, 0, false, false, err
	}
	if hasStart && hasEnd && start > end {
		return 0, 0, false, false, fmt.Errorf("start must not be later than end")
	}
	return start, end, hasStart, hasEnd, nil
}

func parseTimelineRange(dateText string, startText string, endText string) (int64, int64, string, string, error) {
	dateText = strings.TrimSpace(dateText)
	startText = strings.TrimSpace(startText)
	endText = strings.TrimSpace(endText)
	if dateText != "" && (startText != "" || endText != "") {
		return 0, 0, "", "", fmt.Errorf("--date cannot be combined with --start or --end")
	}
	if dateText != "" {
		start, end, err := parseDateRange(dateText)
		if err != nil {
			return 0, 0, "", "", err
		}
		return start, end, formatMetaTime(start, true), formatMetaTime(end, true), nil
	}
	if startText == "" || endText == "" {
		return 0, 0, "", "", fmt.Errorf("timeline requires --date or both --start and --end")
	}
	start, hasStart, err := parseTimeBound(startText, "start", false)
	if err != nil {
		return 0, 0, "", "", err
	}
	end, hasEnd, err := parseTimeBound(endText, "end", true)
	if err != nil {
		return 0, 0, "", "", err
	}
	if !hasStart || !hasEnd {
		return 0, 0, "", "", fmt.Errorf("timeline requires both --start and --end")
	}
	if start > end {
		return 0, 0, "", "", fmt.Errorf("start must not be later than end")
	}
	return start, end, formatMetaTime(start, true), formatMetaTime(end, true), nil
}

func parseDateRange(value string) (int64, int64, error) {
	value = strings.TrimSpace(value)
	now := time.Now()
	switch value {
	case "today":
		year, month, day := now.Date()
		start := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
		return start.Unix(), start.Add(24*time.Hour - time.Second).Unix(), nil
	case "yesterday":
		year, month, day := now.AddDate(0, 0, -1).Date()
		start := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
		return start.Unix(), start.Add(24*time.Hour - time.Second).Unix(), nil
	default:
		start, hasStart, err := parseTimeBound(value, "date", false)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid date %q: use today, yesterday, or YYYY-MM-DD", value)
		}
		end, hasEnd, err := parseTimeBound(value, "date", true)
		if err != nil || !hasStart || !hasEnd {
			return 0, 0, fmt.Errorf("invalid date %q: use today, yesterday, or YYYY-MM-DD", value)
		}
		if end-start != 24*60*60-1 {
			return 0, 0, fmt.Errorf("invalid date %q: use today, yesterday, or YYYY-MM-DD", value)
		}
		return start, end, nil
	}
}

func parseTimeBound(value string, field string, isEnd bool) (int64, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false, nil
	}
	if allDigits(value) {
		ts, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, false, fmt.Errorf("invalid %s time %q", field, value)
		}
		return ts, true, nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.Unix(), true, nil
	}
	layouts := []struct {
		layout   string
		dateOnly bool
	}{
		{"2006-01-02 15:04:05", false},
		{"2006-01-02 15:04", false},
		{"2006-01-02", true},
	}
	for _, item := range layouts {
		t, err := time.ParseInLocation(item.layout, value, time.Local)
		if err != nil {
			continue
		}
		if item.dateOnly && isEnd {
			t = t.Add(24*time.Hour - time.Second)
		}
		return t.Unix(), true, nil
	}
	return 0, false, fmt.Errorf("invalid %s time %q: use Unix seconds, YYYY-MM-DD, YYYY-MM-DD HH:MM, YYYY-MM-DD HH:MM:SS, or RFC3339", field, value)
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func validFormat(format string) bool {
	switch format {
	case "table", "json", "jsonl", "csv":
		return true
	default:
		return false
	}
}

func validKind(kind string) bool {
	switch kind {
	case contacts.KindAll, contacts.KindFriend, contacts.KindChatroom, contacts.KindOther:
		return true
	default:
		return false
	}
}

func validSort(sortBy string) bool {
	switch sortBy {
	case "username", "name":
		return true
	default:
		return false
	}
}

func flagProvided(args []string, name string) bool {
	long := "--" + name
	for _, arg := range args {
		if arg == long || strings.HasPrefix(arg, long+"=") {
			return true
		}
	}
	return false
}

func trimMessagePage(list []messages.Message, limit int) ([]messages.Message, bool) {
	if limit <= 0 || len(list) <= limit {
		return list, false
	}
	return list[:limit], true
}

func formatMetaTime(ts int64, ok bool) string {
	if !ok || ts <= 0 {
		return ""
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04:05")
}

func localTimezoneName() string {
	if tz := strings.TrimSpace(os.Getenv("TZ")); tz != "" {
		return tz
	}
	if time.Local != nil {
		if name := strings.TrimSpace(time.Local.String()); name != "" && name != "Local" {
			return name
		}
	}
	if target, err := os.Readlink("/etc/localtime"); err == nil {
		if idx := strings.LastIndex(target, "/zoneinfo/"); idx >= 0 {
			return strings.TrimPrefix(target[idx+len("/zoneinfo/"):], "/")
		}
	}
	_, offset := time.Now().Zone()
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	return fmt.Sprintf("UTC%s%02d:%02d", sign, offset/3600, (offset%3600)/60)
}

func buildMessagesNextArgs(username string, start int64, hasStart bool, end int64, hasEnd bool, nextAfterSeq int64, limit int, includeSource bool) []string {
	args := []string{"messages", "--username", username}
	if hasStart {
		args = append(args, "--start", formatMetaTime(start, true))
	}
	if hasEnd {
		args = append(args, "--end", formatMetaTime(end, true))
	}
	args = append(args, "--after-seq", strconv.FormatInt(nextAfterSeq, 10))
	if limit > 0 {
		args = append(args, "--limit", strconv.Itoa(limit))
	}
	if includeSource {
		args = append(args, "--source")
	}
	args = append(args, "--format", "json")
	return args
}

func buildTimelineNextArgs(username string, kind string, query string, start string, end string, limit int, cursor string, includeSource bool) []string {
	args := []string{"timeline"}
	if username != "" {
		args = append(args, "--username", username)
	} else {
		args = append(args, "--kind", kind)
		if query != "" {
			args = append(args, "--query", query)
		}
	}
	args = append(args, "--start", start, "--end", end, "--limit", strconv.Itoa(limit), "--cursor", cursor)
	if includeSource {
		args = append(args, "--source")
	}
	args = append(args, "--format", "json")
	return args
}

var defaultContactFields = []string{"username", "alias", "remark", "nick_name", "head_url", "kind"}
var defaultMessageFields = []string{"id", "chat_username", "chat_kind", "chat_display_name", "chat_alias", "chat_remark", "chat_nick_name", "from_username", "direction", "is_self", "is_chatroom", "seq", "server_id", "create_time", "time", "type", "sub_type", "content", "content_detail", "content_encoding"}
var sourceMessageFields = []string{"source_db", "source_table", "source_local_id", "source_raw_type", "source_status", "source_real_sender_id"}

func writeContacts(w io.Writer, list []contacts.Contact, format string) error {
	switch format {
	case "json":
		enc := jsonEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(list)
	case "jsonl":
		enc := jsonEncoder(w)
		for _, contact := range list {
			if err := enc.Encode(contact); err != nil {
				return err
			}
		}
		return nil
	case "csv":
		cw := csv.NewWriter(w)
		if err := cw.Write(defaultContactFields); err != nil {
			return err
		}
		for _, contact := range list {
			if err := cw.Write(contactValues(contact)); err != nil {
				return err
			}
		}
		cw.Flush()
		return cw.Error()
	case "table":
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, strings.ToUpper(strings.Join(defaultContactFields, "\t")))
		for _, contact := range list {
			values := contactValues(contact)
			for i := range values {
				values[i] = cleanCell(values[i])
			}
			fmt.Fprintln(tw, strings.Join(values, "\t"))
		}
		return tw.Flush()
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

func writeMessages(w io.Writer, list []messages.Message, format string, includeSource bool) error {
	fields := messageFields(includeSource)
	switch format {
	case "json":
		enc := jsonEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(list)
	case "jsonl":
		enc := jsonEncoder(w)
		for _, message := range list {
			if err := enc.Encode(message); err != nil {
				return err
			}
		}
		return nil
	case "csv":
		cw := csv.NewWriter(w)
		if err := cw.Write(fields); err != nil {
			return err
		}
		for _, message := range list {
			if err := cw.Write(messageValues(message, fields)); err != nil {
				return err
			}
		}
		cw.Flush()
		return cw.Error()
	case "table":
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, strings.ToUpper(strings.Join(fields, "\t")))
		for _, message := range list {
			values := messageValues(message, fields)
			for i := range values {
				values[i] = cleanCell(values[i])
			}
			fmt.Fprintln(tw, strings.Join(values, "\t"))
		}
		return tw.Flush()
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

func writeMessageEnvelope(w io.Writer, meta any, list []messages.Message) error {
	enc := jsonEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(messageEnvelope{Meta: meta, Items: list})
}

func messageFields(includeSource bool) []string {
	fields := append([]string{}, defaultMessageFields...)
	if includeSource {
		fields = append(fields, sourceMessageFields...)
	}
	return fields
}

func writeCount(w io.Writer, count int, format string) error {
	switch format {
	case "json", "jsonl":
		return jsonEncoder(w).Encode(map[string]int{"count": count})
	case "csv":
		cw := csv.NewWriter(w)
		if err := cw.Write([]string{"count"}); err != nil {
			return err
		}
		if err := cw.Write([]string{fmt.Sprintf("%d", count)}); err != nil {
			return err
		}
		cw.Flush()
		return cw.Error()
	case "table":
		_, err := fmt.Fprintf(w, "%d\n", count)
		return err
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

func contactValues(contact contacts.Contact) []string {
	values := make([]string, 0, len(defaultContactFields))
	for _, field := range defaultContactFields {
		values = append(values, contactValue(contact, field))
	}
	return values
}

func contactValue(contact contacts.Contact, field string) string {
	switch field {
	case "username":
		return contact.Username
	case "alias":
		return contact.Alias
	case "remark":
		return contact.Remark
	case "nick_name":
		return contact.NickName
	case "head_url":
		return contact.HeadURL
	case "kind":
		return contact.Kind
	default:
		return ""
	}
}

func messageValues(message messages.Message, fields []string) []string {
	values := make([]string, 0, len(fields))
	for _, field := range fields {
		values = append(values, messageValue(message, field))
	}
	return values
}

func messageValue(message messages.Message, field string) string {
	switch field {
	case "id":
		return message.ID
	case "chat_username":
		return message.ChatUsername
	case "chat_kind":
		return message.ChatKind
	case "chat_display_name":
		return message.ChatDisplayName
	case "chat_alias":
		return message.ChatAlias
	case "chat_remark":
		return message.ChatRemark
	case "chat_nick_name":
		return message.ChatNickName
	case "from_username":
		return message.FromUsername
	case "direction":
		return message.Direction
	case "is_self":
		return strconv.FormatBool(message.IsSelf)
	case "is_chatroom":
		return strconv.FormatBool(message.IsChatroom)
	case "seq":
		return fmt.Sprintf("%d", message.Seq)
	case "server_id":
		return fmt.Sprintf("%d", message.ServerID)
	case "create_time":
		return fmt.Sprintf("%d", message.CreateTime)
	case "time":
		return message.Time
	case "type":
		return fmt.Sprintf("%d", message.Type)
	case "sub_type":
		return fmt.Sprintf("%d", message.SubType)
	case "content":
		return message.Content
	case "content_detail":
		if message.ContentDetail == nil {
			return ""
		}
		if text := message.ContentDetail["text"]; text != "" {
			return text
		}
		return message.ContentDetail["type"]
	case "content_encoding":
		return message.ContentEncoding
	case "source_db":
		if message.Source != nil {
			return message.Source.DB
		}
		return message.SourceDB
	case "source_table":
		if message.Source != nil {
			return message.Source.Table
		}
		return message.TableName
	case "source_local_id":
		return fmt.Sprintf("%d", message.LocalID)
	case "source_raw_type":
		return fmt.Sprintf("%d", message.RawType)
	case "source_status":
		return fmt.Sprintf("%d", message.Status)
	case "source_real_sender_id":
		return fmt.Sprintf("%d", message.RealSenderID)
	default:
		return ""
	}
}

func jsonEncoder(w io.Writer) *json.Encoder {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc
}

func cleanCell(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
