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
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"weview/internal/app"
	"weview/internal/contacts"
	"weview/internal/daemon"
	"weview/internal/key"
)

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
user's own machine. V1 focuses on contact/contact.db: it can obtain the
database key, decrypt the contact database into ~/.weview/cache, run a local
daemon for near-real-time refresh, and list contacts or contact-table groups.

Commands:
  weview init       First-time setup: detect WeChat, get the contact DB key,
                    and save it locally. Usually run once at the beginning.
  weview daemon     Start the local refresh daemon over ~/.weview/weview.sock.
  weview contacts   List contacts from the decrypted contact cache.
  weview help CMD   Show detailed help for a command.

Common examples:
  sudo weview init
  weview contacts --format json
  weview contacts --kind friend --format jsonl
  weview contacts --kind friend --format csv
  weview contacts --kind friend --query AI --limit 20 --format json
  weview contacts --kind chatroom --format table
  weview contacts --refresh --format json
  weview daemon

Machine-readable usage:
  Use --format json for a JSON array.
  Use --format jsonl for one JSON object per line.
  Use --format csv for spreadsheet/shell pipelines.
  Use --kind friend for ordinary private-chat contacts.
  Use --kind chatroom for groups present in the contact table.

Current V1 scope:
  Supported: macOS WeChat 4.x contact/contact.db.
  Not included yet: message DBs, media/image decoding, WAL patching, public Web API.

Run:
  weview init --help
  weview contacts --help
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
	default:
		usage(stderr)
		return fmt.Errorf("unknown command: %s", command)
	}
	return nil
}

func initUsage(w io.Writer) {
	fmt.Fprintln(w, `weview init - First-time setup for reading local WeChat data

Usage:
  sudo weview init
  sudo go run ./cmd/weview init

When to run:
  Run this at the beginning before using contacts/daemon.
  In normal use, run it once. Run it again only if WeChat changes account,
  contact/contact.db salt changes, the saved key becomes invalid, or WeChat is
  reinstalled/updated in a way that changes the database key.

What it does:
  1. Detects the current macOS WeChat 4.x account under:
     ~/Library/Containers/com.tencent.xinWeChat/Data/Documents/xwechat_files/<account>/db_storage
  2. Reads contact/contact.db page 1 salt.
  3. Reuses an existing valid key from ~/.weview/keys.json, or scans the running
     WeChat process memory for the SQLCipher raw key.
  4. Verifies the key with page 1 HMAC.
  5. Saves metadata and the key in ~/.weview/keys.json with mode 0600.

Output fields:
  account       WeChat account directory name.
  data_dir      Source db_storage directory.
  db_rel_path   contact/contact.db in V1.
  salt          DB salt from page 1.
  fingerprint   Short key fingerprint. The full key is never printed.
  status        reused or scanned. reused means the saved key is still valid.

Notes:
  Key scanning needs WeChat running and macOS permission to read its process
  memory. On Hardened Runtime WeChat builds, sudo alone may not be enough; use a
  local GUI terminal with Developer Tools permission or ad-hoc re-sign WeChat.`)
}

func daemonUsage(w io.Writer) {
	fmt.Fprintln(w, `weview daemon - Run the local WeChat data refresh daemon

Usage:
  weview daemon
  go run ./cmd/weview daemon

What it does:
  1. Ensures the contact DB key exists.
  2. Decrypts contact/contact.db into:
     ~/.weview/cache/<account>/contact/contact.db
  3. Opens an internal Unix socket:
     ~/.weview/weview.sock
  4. Watches the main contact/contact.db file and refreshes the cache after a
     debounce delay when it changes.

Internal daemon actions:
  health
  refresh_contacts

Notes:
  This is an internal local transport, not a public Web API.
  V1 does not patch or stream .db-wal, so refresh is near-real-time after WeChat
  checkpoints/writes the main DB.
  Stop with Ctrl-C.`)
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
	if err := fs.Parse(args); err != nil {
		return err
	}
	res, err := key.EnsureContactKey(ctx)
	if err != nil {
		return err
	}
	status := "scanned"
	if res.Reused {
		status = "reused"
	}
	fmt.Fprintf(stdout, "account: %s\n", res.Target.Account)
	fmt.Fprintf(stdout, "data_dir: %s\n", res.Target.DataDir)
	fmt.Fprintf(stdout, "db_rel_path: %s\n", res.Target.DBRelPath)
	fmt.Fprintf(stdout, "salt: %s\n", res.Entry.Salt)
	fmt.Fprintf(stdout, "fingerprint: %s\n", res.Entry.Fingerprint)
	fmt.Fprintf(stdout, "status: %s\n", status)
	return nil
}

func runDaemon(args []string, stdout io.Writer) error {
	if hasHelp(args) {
		daemonUsage(stdout)
		return nil
	}
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}
	socketPath, err := app.SocketPath()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Fprintf(stdout, "daemon socket: %s\n", socketPath)
	fmt.Fprintln(stdout, "initializing contact cache...")
	server := &daemon.Server{SocketPath: socketPath}
	if err := server.Run(ctx); err != nil {
		return err
	}
	return nil
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

var defaultContactFields = []string{"username", "alias", "remark", "nick_name", "head_url", "kind"}

func writeContacts(w io.Writer, list []contacts.Contact, format string) error {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(list)
	case "jsonl":
		enc := json.NewEncoder(w)
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

func writeCount(w io.Writer, count int, format string) error {
	switch format {
	case "json", "jsonl":
		return json.NewEncoder(w).Encode(map[string]int{"count": count})
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

func cleanCell(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
