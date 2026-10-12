// Command q41eu-dev is the dev app for Stackchan robots in Embody Mode: robots connect to it as
// to any app, a browser pairs by the robot's QR code and makes API keys, and programs and AI
// agents use the robot through its higher-level API (/v1, and MCP at /mcp).
//
// Robots connect to ws://<host>/api/devices/connect with a bearer token (by default the same token
// file as q41eu-raw), or with a token of their own from a Stackchan manager (-manager-url).
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/i41-eu/q41eu-dev/internal/devapp"
	"github.com/i41-eu/q41eu-raw/robotauth"
	"github.com/i41-eu/q41eu-raw/sso"
	"github.com/i41-eu/q41eu-raw/statestore"
)

var version = "dev" // set by the release build

func main() {
	var (
		listen       = flag.String("listen", ":8785", "HTTP listen address for robots, browsers and API clients")
		publicURL    = flag.String("public-url", "", "base URL browsers and programs use to reach this server (default: http://<LAN IP>:<port>)")
		tokenFile    = flag.String("token-file", defaultConfigFile("stackchan-server", "robot-token"), "file with the robot bearer token; generated if missing")
		stateFile    = flag.String("state-file", defaultStateFile(), "JSON file with pairings and keys (\"\" keeps them in memory only)")
		stateDB      = flag.String("state-database", os.Getenv("STATE_DATABASE_URL"), "keep the state in Postgres instead (postgres://user@host/db; the password from PGPASSWORD)")
		uiDir        = flag.String("ui-dir", "", "development: render the site from this directory on every request (internal/devapp/site)")
		debug        = flag.Bool("debug", false, "debug logging")
		mgrURL       = flag.String("manager-url", "", "the Stackchan manager that set robots up with tokens for this app (\"\" = none)")
		mgrSecret    = flag.String("manager-secret-file", "", "file with this app's secret at the manager")
		mgrSignIn    = flag.Bool("manager-sign-in", false, "sign in through the manager (-manager-url): the owner's browsers get their robots without the QR code")
		signInAlways = flag.Bool("sign-in-always", false, "with -manager-sign-in: on Your robots (and Run without a robot) try a silent sign-in at the manager also without its hint cookie (this app on another domain than the manager's -sign-in-hint-domain), at most once per 10 minutes per browser; the docs pages never redirect")
		proxies      = flag.Int("trusted-proxies", 0, "reverse proxies in front of this server that append the client address to X-Forwarded-For (0: trust no header)")
	)
	flag.Parse()
	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	fail := func(msg string, err error) {
		log.Error(msg, "err", err)
		os.Exit(1)
	}
	token, err := loadOrCreateToken(*tokenFile, log)
	if err != nil {
		fail("robot token", err)
	}
	if *publicURL == "" {
		if *publicURL, err = defaultPublicURL(*listen); err != nil {
			fail("public URL", err)
		}
	}
	var manager *robotauth.Client
	var signIn *sso.Client
	if *mgrURL != "" {
		b, err := os.ReadFile(*mgrSecret)
		if err != nil || strings.TrimSpace(string(b)) == "" {
			fail("manager secret file", fmt.Errorf("%s: %v", *mgrSecret, err))
		}
		manager = robotauth.New(*mgrURL, strings.TrimSpace(string(b)))
		if *mgrSignIn {
			signIn = sso.New(*mgrURL, strings.TrimSpace(string(b)))
		}
	} else if *mgrSignIn {
		fail("-manager-sign-in", errors.New("needs -manager-url and -manager-secret-file"))
	}
	var state statestore.Store
	if *stateDB != "" || *stateFile != "" {
		openCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		var from *statestore.File
		if *stateFile != "" {
			if err := os.MkdirAll(filepath.Dir(*stateFile), 0o700); err != nil {
				fail("state directory", err)
			}
			from = &statestore.File{Path: *stateFile}
		}
		st, err := statestore.Open(openCtx, *stateDB, "dev", from)
		cancel()
		if err != nil {
			fail("state", err)
		}
		state = st
		log.Info("state", "where", st.Where())
	}
	a := devapp.New(devapp.Config{RobotToken: token, Manager: manager, SignIn: signIn, SignInAlways: *signInAlways, ManagerURL: strings.TrimRight(*mgrURL, "/"), PublicURL: strings.TrimRight(*publicURL, "/"),
		State: state, UIDir: *uiDir, TrustedProxies: *proxies, Log: log, Version: version})
	// No write timeout: actions like ask wait up to two minutes for a person.
	srv := &http.Server{Addr: *listen, Handler: a.Handler(), ReadHeaderTimeout: 10 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go a.Run(ctx)
	if state != nil {
		go func() {
			select {
			case <-state.Lost():
				log.Error("state database: connection (and lock) lost; exiting")
				os.Exit(1)
			case <-ctx.Done():
			}
		}()
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()
	log.Info("q41eu-dev listening", "listen", *listen, "public_url", *publicURL, "manager", *mgrURL, "version", version)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fail("server", err)
	}
	if err := a.Save(context.Background()); err != nil {
		log.Warn("state not saved", "err", err)
	}
	if state != nil {
		state.Close()
	}
}

// defaultStateFile follows the XDG base directory spec: $XDG_STATE_HOME or ~/.local/state.
func defaultStateFile() string {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "q41eu-dev", "state.json")
}

func defaultConfigFile(app, name string) string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return name
	}
	return filepath.Join(dir, app, name)
}

// loadOrCreateToken reads the robot token, generating a random one on first run.
func loadOrCreateToken(path string, log *slog.Logger) (string, error) {
	b, err := os.ReadFile(path)
	if err == nil {
		token := strings.TrimSpace(string(b))
		if token == "" {
			return "", fmt.Errorf("%s is empty", path)
		}
		return token, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	raw := make([]byte, 32)
	rand.Read(raw)
	token := hex.EncodeToString(raw)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return "", err
	}
	log.Info("generated new robot token", "path", path)
	return token, nil
}

// defaultPublicURL builds http://<LAN IP>:<port> so a phone on the same network can open the QR link.
func defaultPublicURL(listen string) (string, error) {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", fmt.Errorf("parse -listen %q: %w", listen, err)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = lanIP()
	}
	return "http://" + net.JoinHostPort(host, port), nil
}

// lanIP returns the address of the interface used for outbound traffic (no packets are sent).
func lanIP() string {
	if c, err := net.Dial("udp4", "192.0.2.1:80"); err == nil {
		defer c.Close()
		return c.LocalAddr().(*net.UDPAddr).IP.String()
	}
	return "127.0.0.1"
}
