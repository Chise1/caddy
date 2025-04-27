package logging

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
)

type Syslog struct {
	Ts     int64  `json:"ts,omitempty"`
	Level  string `json:"level,omitempty"`
	Logger string `json:"logger,omitempty"`
	Type   string `json:"type,omitempty"`
}

func init() {
	caddy.RegisterModule(HttpWriter{})
}

// HttpWriter implements a log writer that outputs to a network socket. If
// the socket goes down, it will dump logs to stderr while it attempts to
// reconnect.
type HttpWriter struct {
	Url    string `json:"url,omitempty"`
	Key    string `json:"key,omitempty"`
	Count  int    `json:"count,omitempty"`
	Period int    `json:"period,omitempty"`
	Value  string `json:"value,omitempty"`
}

// CaddyModule returns the Caddy module information.
func (HttpWriter) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "caddy.logging.writers.http",
		New: func() caddy.Module { return new(HttpWriter) },
	}
}

// Provision sets up the module.
func (nw *HttpWriter) Provision(ctx caddy.Context) error {
	repl := caddy.NewReplacer()
	_, err := repl.ReplaceOrErr(nw.Url, true, true)
	if err != nil {
		return fmt.Errorf("invalid host in url: %v", err)
	}
	return nil
}

func (nw HttpWriter) String() string {
	return nw.Url
}

// WriterKey returns a unique key representing this nw.
func (nw HttpWriter) WriterKey() string {
	return nw.Url
}

// OpenWriter opens a new network connection.
func (nw HttpWriter) OpenWriter() (io.WriteCloser, error) {
	reconn := &httpConn{
		httpWriter: nw,
		client:     &http.Client{},
	}
	return reconn, nil
}

func (nw *HttpWriter) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		if !d.NextArg() {
			return d.ArgErr()
		}
		nw.Url = d.Val()
		if d.NextArg() {
			return d.ArgErr()
		}
		for nesting := d.Nesting(); d.NextBlock(nesting); {
			switch d.Val() {
			case "count":
				if !d.NextArg() {
					return d.ArgErr()
				}
				count, err := strconv.Atoi(d.Val())
				if err != nil {
					return d.Errf("invalid int: %s", d.Val())
				}
				if d.NextArg() {
					return d.ArgErr()
				}
				nw.Count = count
			case "period":
				if !d.NextArg() {
					return d.ArgErr()
				}
				period, err := strconv.Atoi(d.Val())
				if err != nil {
					return d.Errf("invalid int: %s", d.Val())
				}
				if d.NextArg() {
					return d.ArgErr()
				}
				nw.Period = period
			case "key":
				if !d.NextArg() {
					return d.ArgErr()
				}
				nw.Key = d.Val()
			case "value":
				if !d.NextArg() {
					return d.ArgErr()
				}
				nw.Value = d.Val()
			default: //TODO need test
				return d.Errf("got error data:%s", d.Val())
			}
		}
	}
	return nil
}

// httpConn wraps an underlying Conn so that if any
// writes fail, the connection is redialed and the write
// is retried.
type httpConn struct {
	httpWriter HttpWriter
	client     *http.Client
}

func (reconn *httpConn) Close() error {
	return nil
}

// Write wraps the underlying Conn.Write method, but if that fails,
// it will re-dial the connection anew and try writing again.
func (reconn *httpConn) Write(b []byte) (n int, err error) {
	if b[len(b)-1] == '\n' {
		b = b[0 : len(b)-1]
	}
	var requestInfo []byte
	syslogBytes := bytes.Split(b, []byte("\t"))
	if len(syslogBytes) != 5 {
		requestInfo = b
	} else {
		requestInfo = syslogBytes[4]
	}
	if err != nil {
		return 0, err
	}
	body := bytes.NewBuffer(requestInfo)
	// Print the request body for debugging
	req, err := http.NewRequest("POST", reconn.httpWriter.Url, body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if len(reconn.httpWriter.Key) > 0 {
		req.Header.Add(reconn.httpWriter.Key, reconn.httpWriter.Value)
	}
	resp, err := reconn.client.Do(req)
	if err != nil {
		return 0, err
	}
	if !(resp.StatusCode >= 200 && resp.StatusCode < 300) {
		b := fmt.Sprintf("%#v", resp)
		err = errors.New(b)
	}
	return len(b), nil
}

// Interface guards
var (
	_ caddy.Provisioner     = (*HttpWriter)(nil)
	_ caddy.WriterOpener    = (*HttpWriter)(nil)
	_ caddyfile.Unmarshaler = (*HttpWriter)(nil)
)
