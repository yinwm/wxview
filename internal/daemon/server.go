package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"weview/internal/app"
	"weview/internal/key"
)

type Server struct {
	SocketPath string

	mu     sync.Mutex
	target key.TargetDB
}

func (s *Server) Run(ctx context.Context) error {
	if s.SocketPath == "" {
		return fmt.Errorf("socket path is empty")
	}
	if err := s.refresh(ctx); err != nil {
		return err
	}

	if err := s.prepareSocket(ctx); err != nil {
		return err
	}
	listener, err := net.Listen("unix", s.SocketPath)
	if err != nil {
		return err
	}
	defer listener.Close()
	defer os.Remove(s.SocketPath)
	_ = os.Chmod(s.SocketPath, 0o600)
	if err := app.ChownForSudo(s.SocketPath); err != nil {
		return err
	}

	go s.watchContacts(ctx)

	errCh := make(chan error, 1)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				errCh <- err
				return
			}
			go s.handleConn(ctx, conn)
		}
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) prepareSocket(ctx context.Context) error {
	client := Client{SocketPath: s.SocketPath, Timeout: 500 * time.Millisecond}
	if client.Healthy(ctx) {
		return fmt.Errorf("daemon already running at %s", s.SocketPath)
	}
	if err := os.Remove(s.SocketPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *Server) refresh(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, _, err := key.EnsureContactCache(ctx)
	if err != nil {
		return err
	}
	s.target = res.Target
	return nil
}

func (s *Server) watchContacts(ctx context.Context) {
	s.mu.Lock()
	path := s.target.DBPath
	s.mu.Unlock()
	if path == "" {
		return
	}
	WatchFile(ctx, path, time.Second, time.Second, func() {
		if err := s.refresh(ctx); err != nil {
			log.Printf("refresh contact cache failed: %v", err)
			return
		}
		log.Printf("contact cache refreshed")
	})
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	var req Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		_ = json.NewEncoder(conn).Encode(Response{OK: false, Message: err.Error()})
		return
	}
	switch req.Action {
	case ActionHealth:
		_ = json.NewEncoder(conn).Encode(Response{OK: true, Message: "ok"})
	case ActionRefreshContacts:
		if err := s.refresh(ctx); err != nil {
			_ = json.NewEncoder(conn).Encode(Response{OK: false, Message: err.Error()})
			return
		}
		_ = json.NewEncoder(conn).Encode(Response{OK: true, Message: "refreshed"})
	default:
		_ = json.NewEncoder(conn).Encode(Response{OK: false, Message: "unknown daemon action: " + req.Action})
	}
}
