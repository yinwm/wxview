package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"time"
)

type Client struct {
	SocketPath string
	Timeout    time.Duration
}

func (c Client) Call(ctx context.Context, action string) (Response, error) {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 2 * time.Second
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "unix", c.SocketPath)
	if err != nil {
		return Response{}, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))
	if err := json.NewEncoder(conn).Encode(Request{Action: action}); err != nil {
		return Response{}, err
	}
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return Response{}, err
	}
	if !resp.OK {
		if resp.Message == "" {
			resp.Message = "daemon request failed"
		}
		return resp, errors.New(resp.Message)
	}
	return resp, nil
}

func (c Client) Healthy(ctx context.Context) bool {
	resp, err := c.Call(ctx, ActionHealth)
	return err == nil && resp.OK
}
