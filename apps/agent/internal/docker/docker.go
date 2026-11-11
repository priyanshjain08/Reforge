// Package docker talks to the Docker Engine API over its unix socket using only the standard library.
package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"time"
)

var nameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}$`)

type Client struct {
	http *http.Client
}

func New(socket string) *Client {
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", socket)
	}}
	return &Client{http: &http.Client{Transport: tr, Timeout: 40 * time.Second}}
}

type Info struct {
	Name     string
	State    string
	Status   string
	Health   string
	Restarts int
	ExitCode int
}

func (c *Client) Ping(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/_ping", nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) Inspect(ctx context.Context, name string) (Info, error) {
	if !nameRe.MatchString(name) {
		return Info{}, fmt.Errorf("invalid container name")
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/containers/"+name+"/json", nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return Info{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return Info{Name: name, State: "missing", Status: "container not found"}, nil
	}
	if resp.StatusCode != 200 {
		return Info{}, fmt.Errorf("docker inspect status %d", resp.StatusCode)
	}
	var body struct {
		RestartCount int
		State        struct {
			Status   string
			ExitCode int
			Health   *struct{ Status string }
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Info{}, err
	}
	in := Info{Name: name, State: body.State.Status, Status: body.State.Status, Restarts: body.RestartCount, ExitCode: body.State.ExitCode}
	if body.State.Health != nil {
		in.Health = body.State.Health.Status
	}
	return in, nil
}

func (c *Client) Restart(ctx context.Context, name string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("invalid container name")
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://docker/containers/"+name+"/restart?t=5", nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		return fmt.Errorf("docker restart returned status %d", resp.StatusCode)
	}
	return nil
}
