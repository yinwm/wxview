package main

import (
	"context"
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
	case "init", "key":
		return runKey(ctx, args[1:], stdout)
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
  weview contacts --kind chatroom --format table
  weview contacts --refresh --format json
  weview daemon

Machine-readable usage:
  Use --format json for a JSON array.
  Use --format jsonl for one JSON object per line.
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
	case "init", "key":
		keyUsage(stdout)
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

func keyUsage(w io.Writer) {
	fmt.Fprintln(w, `weview init - First-time setup for reading local WeChat data

Usage:
  sudo weview init
  sudo go run ./cmd/weview init

Compatibility:
  sudo weview key

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
  list_contacts

Notes:
  This is an internal local transport, not a public Web API.
  V1 does not patch or stream .db-wal, so refresh is near-real-time after WeChat
  checkpoints/writes the main DB.
  Stop with Ctrl-C.`)
}

func contactsUsage(w io.Writer) {
	fmt.Fprintln(w, `weview contacts - List WeChat contacts and contact-table groups

Usage:
  weview contacts [--format table|json|jsonl] [--kind all|friend|chatroom|other] [--refresh]
  weview contact  [--format table|json|jsonl] [--kind all|friend|chatroom|other] [--refresh]

Flags:
  --format table   Human-readable table output. This is the default.
  --format json    Machine-readable JSON array.
  --format jsonl   Machine-readable newline-delimited JSON, one contact per line.

  --kind all       Return every row selected from the contact table. Default.
  --kind friend    Ordinary private-chat contacts:
                   local_type = 1, username not ending in @chatroom, username not starting with gh_.
  --kind chatroom  Groups visible in the contact table:
                   username ending in @chatroom.
  --kind other     Official accounts, enterprise contacts, non-friend room members,
                   and special/system contacts.

  --refresh        Before listing, decrypt the source contact/contact.db into the
                   local cache again. Without --refresh, uses the existing cache
                   when available.

Output fields:
  username    Stable WeChat username, e.g. wxid_* or *@chatroom.
  alias       WeChat ID / alias when present.
  remark      Your remark for the contact.
  nick_name   Contact nickname from WeChat.
  head_url    Raw big_head_url from contact.db.
  kind        friend, chatroom, or other.

Examples for humans:
  weview contacts
  weview contacts --kind friend
  weview contacts --kind chatroom

Examples for AI/tools:
  weview contacts --kind friend --format json
  weview contacts --kind chatroom --format jsonl
  weview contacts --refresh --format json

Runtime behavior:
  If the daemon is running, this command uses ~/.weview/weview.sock.
  If the daemon is not running, it reads or refreshes the local decrypted cache
  directly.`)
}

func hasHelp(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "help" {
			return true
		}
	}
	return false
}

func runKey(ctx context.Context, args []string, stdout io.Writer) error {
	if hasHelp(args) {
		keyUsage(stdout)
		return nil
	}
	fs := flag.NewFlagSet("key", flag.ContinueOnError)
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
	if hasHelp(args) {
		contactsUsage(stdout)
		return nil
	}
	fs := flag.NewFlagSet("contacts", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	format := fs.String("format", "table", "table, json, or jsonl")
	kind := fs.String("kind", contacts.KindAll, "all, friend, chatroom, or other")
	refresh := fs.Bool("refresh", false, "refresh decrypted contact cache before listing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !validFormat(*format) {
		return fmt.Errorf("invalid format %q: use table, json, or jsonl", *format)
	}
	if !validKind(*kind) {
		return fmt.Errorf("invalid kind %q: use all, friend, chatroom, or other", *kind)
	}

	list, err := listContacts(ctx, *refresh)
	if err != nil {
		return err
	}
	list = contacts.FilterByKind(list, *kind)
	return writeContacts(stdout, list, *format)
}

func listContacts(ctx context.Context, refresh bool) ([]contacts.Contact, error) {
	socketPath, err := app.SocketPath()
	if err != nil {
		return nil, err
	}
	client := daemon.Client{SocketPath: socketPath, Timeout: 5 * time.Second}
	if client.Healthy(ctx) {
		if refresh {
			if _, err := client.Call(ctx, daemon.ActionRefreshContacts); err != nil {
				return nil, err
			}
		}
		resp, err := client.Call(ctx, daemon.ActionListContacts)
		if err != nil {
			return nil, err
		}
		return resp.Contacts, nil
	}

	var cachePath string
	if refresh {
		_, path, err := key.EnsureContactCache(ctx)
		if err != nil {
			return nil, err
		}
		cachePath = path
	} else if _, path, ok := key.HasContactCache(); ok {
		cachePath = path
	} else {
		_, path, err := key.EnsureContactCache(ctx)
		if err != nil {
			return nil, err
		}
		cachePath = path
	}
	return contacts.NewService(cachePath).List(ctx)
}

func validFormat(format string) bool {
	switch format {
	case "table", "json", "jsonl":
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
	case "table":
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "USERNAME\tALIAS\tREMARK\tNICK_NAME\tHEAD_URL\tKIND")
		for _, contact := range list {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				cleanCell(contact.Username),
				cleanCell(contact.Alias),
				cleanCell(contact.Remark),
				cleanCell(contact.NickName),
				cleanCell(contact.HeadURL),
				cleanCell(contact.Kind),
			)
		}
		return tw.Flush()
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

func cleanCell(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
