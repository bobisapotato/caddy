// Package websockets implements a WebSocket server by executing
// a command and piping its input and output through the WebSocket
// connection.
package websockets

import (
	"errors"
	"net/http"

	"github.com/flynn/go-shlex"
	"github.com/mholt/caddy/middleware"
	"golang.org/x/net/websocket"
)

type (
	// WebSockets is a type that holds configuration for the
	// websocket middleware generally, like a list of all the
	// websocket endpoints.
	WebSockets struct {
		// Next is the next HTTP handler in the chain for when the path doesn't match
		Next http.HandlerFunc

		// Sockets holds all the web socket endpoint configurations
		Sockets []WSConfig
	}

	// WSConfig holds the configuration for a single websocket
	// endpoint which may serve multiple websocket connections.
	WSConfig struct {
		Path      string
		Command   string
		Arguments []string
		Respawn   bool // TODO: Not used, but parser supports it until we decide on it
	}
)

// ServeHTTP converts the HTTP request to a WebSocket connection and serves it up.
func (ws WebSockets) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for _, sockconfig := range ws.Sockets {
		if middleware.Path(r.URL.Path).Matches(sockconfig.Path) {
			socket := WebSocket{
				WSConfig: sockconfig,
				Request:  r,
			}
			websocket.Handler(socket.Handle).ServeHTTP(w, r)
			return
		}
	}

	// Didn't match a websocket path, so pass-thru
	ws.Next(w, r)
}

// New constructs and configures a new websockets middleware instance.
func New(c middleware.Controller) (middleware.Middleware, error) {
	var websocks []WSConfig
	var respawn bool

	optionalBlock := func() (hadBlock bool, err error) {
		for c.NextBlock() {
			hadBlock = true
			if c.Val() == "respawn" {
				respawn = true
			} else {
				return true, c.Err("Expected websocket configuration parameter in block")
			}
		}
		return
	}

	for c.Next() {
		var val, path, command string

		// Path or command; not sure which yet
		if !c.NextArg() {
			return nil, c.ArgErr()
		}
		val = c.Val()

		// Extra configuration may be in a block
		hadBlock, err := optionalBlock()
		if err != nil {
			return nil, err
		}

		if !hadBlock {
			// The next argument on this line will be the command or an open curly brace
			if c.NextArg() {
				path = val
				command = c.Val()
			} else {
				path = "/"
				command = val
			}

			// Okay, check again for optional block
			hadBlock, err = optionalBlock()
			if err != nil {
				return nil, err
			}
		}

		// Split command into the actual command and its arguments
		cmd, args, err := parseCommandAndArgs(command)
		if err != nil {
			return nil, err
		}

		websocks = append(websocks, WSConfig{
			Path:      path,
			Command:   cmd,
			Arguments: args,
			Respawn:   respawn,
		})
	}

	GatewayInterface = envGatewayInterface
	ServerSoftware = envServerSoftware

	return func(next http.HandlerFunc) http.HandlerFunc {
		return WebSockets{Next: next, Sockets: websocks}.ServeHTTP
	}, nil
}

// parseCommandAndArgs takes a command string and parses it
// shell-style into the command and its separate arguments.
func parseCommandAndArgs(command string) (cmd string, args []string, err error) {
	parts, err := shlex.Split(command)
	if err != nil {
		err = errors.New("Error parsing command for websocket: " + err.Error())
		return
	} else if len(parts) == 0 {
		err = errors.New("No command found for use by websocket")
		return
	}

	cmd = parts[0]
	if len(parts) > 1 {
		args = parts[1:]
	}

	return
}

var (
	// See CGI spec, 4.1.4
	GatewayInterface string

	// See CGI spec, 4.1.17
	ServerSoftware string
)

const (
	envGatewayInterface = "caddy-CGI/1.1"
	envServerSoftware   = "caddy/?.?.?" // TODO
)
