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
		usage(stderr)
		return flag.ErrHelp
	}
	ctx := context.Background()
	switch args[0] {
	case "key":
		return runKey(ctx, args[1:], stdout)
	case "daemon":
		return runDaemon(args[1:], stdout)
	case "contacts":
		return runContacts(ctx, args[1:], stdout)
	case "help", "-h", "--help":
		usage(stdout)
		return nil
	default:
		usage(stderr)
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, `weview - WeChat contact decrypt helper

Usage:
  weview key
  weview daemon
  weview contacts [--format table|json|jsonl] [--refresh]

V1 supports macOS WeChat 4.x contact/contact.db only.`)
}

func runKey(ctx context.Context, args []string, stdout io.Writer) error {
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
	fs := flag.NewFlagSet("contacts", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	format := fs.String("format", "table", "table, json, or jsonl")
	refresh := fs.Bool("refresh", false, "refresh decrypted contact cache before listing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !validFormat(*format) {
		return fmt.Errorf("invalid format %q: use table, json, or jsonl", *format)
	}

	list, err := listContacts(ctx, *refresh)
	if err != nil {
		return err
	}
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
		fmt.Fprintln(tw, "USERNAME\tALIAS\tREMARK\tNICK_NAME\tHEAD_URL\tIS_FRIEND")
		for _, contact := range list {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%t\n",
				cleanCell(contact.Username),
				cleanCell(contact.Alias),
				cleanCell(contact.Remark),
				cleanCell(contact.NickName),
				cleanCell(contact.HeadURL),
				contact.IsFriend,
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
