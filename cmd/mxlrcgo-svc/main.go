package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	arg "github.com/alexflint/go-arg"
	"github.com/joho/godotenv"
	"github.com/sydlexius/mxlrcgo-svc/internal/app"
	"github.com/sydlexius/mxlrcgo-svc/internal/auth"
	"github.com/sydlexius/mxlrcgo-svc/internal/cache"
	"github.com/sydlexius/mxlrcgo-svc/internal/config"
	"github.com/sydlexius/mxlrcgo-svc/internal/db"
	"github.com/sydlexius/mxlrcgo-svc/internal/library"
	"github.com/sydlexius/mxlrcgo-svc/internal/lyrics"
	"github.com/sydlexius/mxlrcgo-svc/internal/models"
	"github.com/sydlexius/mxlrcgo-svc/internal/musixmatch"
	"github.com/sydlexius/mxlrcgo-svc/internal/queue"
	"github.com/sydlexius/mxlrcgo-svc/internal/scan"
	"github.com/sydlexius/mxlrcgo-svc/internal/scanner"
	"github.com/sydlexius/mxlrcgo-svc/internal/server"
	"github.com/sydlexius/mxlrcgo-svc/internal/worker"
)

const defaultScanInterval = 15 * time.Minute

// Args defines the CLI arguments for the application.
type Args struct {
	Fetch   *FetchCmd   `arg:"subcommand:fetch" help:"fetch lyrics once without HTTP server or DB queue"`
	Serve   *ServeCmd   `arg:"subcommand:serve" help:"run HTTP server, worker, and library scheduler"`
	Scan    *ScanCmd    `arg:"subcommand:scan" help:"scan configured libraries and enqueue missing lyrics"`
	Library *LibraryCmd `arg:"subcommand:library" help:"manage library roots"`
	Keys    *KeysCmd    `arg:"subcommand:keys" help:"manage API keys"`
	Config  *ConfigCmd  `arg:"subcommand:config" help:"inspect or update configuration"`
}

// LegacyArgs preserves the pre-subcommand CLI surface.
type LegacyArgs struct {
	Song       []string `arg:"positional" help:"song information in [ artist,title ] format, a .txt file, or a directory path"`
	Outdir     *string  `arg:"-o,--outdir" help:"output directory (default: from config or 'lyrics')"`
	Cooldown   *int     `arg:"-c,--cooldown" help:"cooldown time in seconds (default: from config or 15)"`
	Depth      int      `arg:"-d,--depth" help:"(directory mode) maximum recursion depth" default:"100"`
	Update     bool     `arg:"-u,--update" help:"(directory mode) re-fetch and overwrite existing .lrc files"`
	Upgrade    bool     `arg:"--upgrade" help:"(directory mode) re-fetch songs with .txt (unsynced) to promote to .lrc if synced lyrics are now available; implied by --update"`
	BFS        bool     `arg:"--bfs" help:"(directory mode) use breadth-first-search traversal"`
	Serve      bool     `arg:"--serve" help:"run HTTP server mode"`
	Listen     *string  `arg:"--listen" help:"HTTP listen address (default: from config or 127.0.0.1:3876)"`
	Token      string   `arg:"-t,--token" help:"musixmatch token (or MUSIXMATCH_TOKEN / MXLRC_API_TOKEN env var, or config file)" default:""`
	ConfigPath string   `arg:"--config" help:"path to config file (default: XDG)" default:""`
}

// FetchCmd runs the legacy one-shot fetch path.
type FetchCmd struct {
	Song       []string `arg:"positional,required" help:"song information in [ artist,title ] format, a .txt file, or a directory path"`
	Outdir     *string  `arg:"-o,--outdir" help:"output directory (default: from config or 'lyrics')"`
	Cooldown   *int     `arg:"-c,--cooldown" help:"cooldown time in seconds (default: from config or 15)"`
	Depth      int      `arg:"-d,--depth" help:"(directory mode) maximum recursion depth" default:"100"`
	Update     bool     `arg:"-u,--update" help:"(directory mode) re-fetch and overwrite existing .lrc files"`
	Upgrade    bool     `arg:"--upgrade" help:"(directory mode) re-fetch songs with .txt lyrics to promote to .lrc"`
	BFS        bool     `arg:"--bfs" help:"(directory mode) use breadth-first-search traversal"`
	Token      string   `arg:"-t,--token" help:"musixmatch token" default:""`
	ConfigPath string   `arg:"--config" help:"path to config file (default: XDG)" default:""`
}

// ServeCmd runs the daemon.
type ServeCmd struct {
	Listen       *string `arg:"--listen" help:"HTTP listen address (default: from config or 127.0.0.1:3876)"`
	Outdir       *string `arg:"-o,--outdir" help:"output directory (default: from config or 'lyrics')"`
	Token        string  `arg:"-t,--token" help:"musixmatch token" default:""`
	ConfigPath   string  `arg:"--config" help:"path to config file (default: XDG)" default:""`
	Depth        int     `arg:"-d,--depth" help:"scheduler maximum recursion depth" default:"100"`
	Update       bool    `arg:"-u,--update" help:"scheduler re-fetches existing .lrc files"`
	Upgrade      bool    `arg:"--upgrade" help:"scheduler re-fetches .txt lyrics to promote them"`
	BFS          bool    `arg:"--bfs" help:"scheduler uses breadth-first traversal"`
	ScanInterval int     `arg:"--scan-interval" help:"scheduler interval in seconds (default: 900; 0 disables repeat)" default:"900"`
	WorkInterval int     `arg:"--work-interval" help:"worker poll interval in seconds" default:"5"`
}

// ScanCmd scans libraries once and enqueues cache misses.
type ScanCmd struct {
	ConfigPath string `arg:"--config" help:"path to config file (default: XDG)" default:""`
	Depth      int    `arg:"-d,--depth" help:"maximum recursion depth" default:"100"`
	Update     bool   `arg:"-u,--update" help:"re-fetch and overwrite existing .lrc files"`
	Upgrade    bool   `arg:"--upgrade" help:"re-fetch .txt lyrics to promote them"`
	BFS        bool   `arg:"--bfs" help:"use breadth-first traversal"`
}

// LibraryCmd contains nested library subcommands.
type LibraryCmd struct {
	Add    *LibraryAddCmd    `arg:"subcommand:add" help:"add a library root"`
	List   *LibraryListCmd   `arg:"subcommand:list" help:"list library roots"`
	Remove *LibraryRemoveCmd `arg:"subcommand:remove" help:"remove a library root"`
}

type LibraryAddCmd struct {
	Path       string `arg:"positional,required" help:"library root path"`
	Name       string `arg:"--name" help:"display name (default: directory base)" default:""`
	ConfigPath string `arg:"--config" help:"path to config file (default: XDG)" default:""`
}

type LibraryListCmd struct {
	ConfigPath string `arg:"--config" help:"path to config file (default: XDG)" default:""`
}

type LibraryRemoveCmd struct {
	ID         int64  `arg:"positional,required" help:"library id"`
	ConfigPath string `arg:"--config" help:"path to config file (default: XDG)" default:""`
}

// KeysCmd contains nested key subcommands.
type KeysCmd struct {
	Create *KeysCreateCmd `arg:"subcommand:create" help:"create an API key"`
	List   *KeysListCmd   `arg:"subcommand:list" help:"list API keys"`
	Revoke *KeysRevokeCmd `arg:"subcommand:revoke" help:"revoke an API key"`
}

type KeysCreateCmd struct {
	Name       string   `arg:"--name" help:"key name" default:""`
	Scopes     []string `arg:"--scope,separate" help:"scope to grant: webhook or admin"`
	ConfigPath string   `arg:"--config" help:"path to config file (default: XDG)" default:""`
}

type KeysListCmd struct {
	ConfigPath string `arg:"--config" help:"path to config file (default: XDG)" default:""`
}

type KeysRevokeCmd struct {
	Key        string `arg:"positional,required" help:"raw API key to revoke"`
	ConfigPath string `arg:"--config" help:"path to config file (default: XDG)" default:""`
}

// ConfigCmd contains nested config subcommands.
type ConfigCmd struct {
	Get  *ConfigGetCmd  `arg:"subcommand:get" help:"get a config value"`
	Set  *ConfigSetCmd  `arg:"subcommand:set" help:"set a config value"`
	List *ConfigListCmd `arg:"subcommand:list" help:"list config values"`
}

type ConfigGetCmd struct {
	Key        string `arg:"positional,required" help:"config key"`
	ConfigPath string `arg:"--config" help:"path to config file (default: XDG)" default:""`
}

type ConfigSetCmd struct {
	Key        string `arg:"positional,required" help:"config key"`
	Value      string `arg:"positional,required" help:"config value"`
	ConfigPath string `arg:"--config" help:"path to config file (default: XDG)" default:""`
}

type ConfigListCmd struct {
	ConfigPath string `arg:"--config" help:"path to config file (default: XDG)" default:""`
}

func main() {
	os.Exit(run())
}

type appRunner interface {
	Run(ctx context.Context) error
}

type runOptions struct {
	args       []string
	out        io.Writer
	ctx        context.Context
	loadDotenv func() error
	newFetcher func(token string) musixmatch.Fetcher
	newWriter  func() lyrics.Writer
	newApp     func(fetcher musixmatch.Fetcher, writer lyrics.Writer, inputs *queue.InputsQueue, cooldown int, mode string) appRunner
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return runWithOptions(runOptions{ctx: ctx})
}

func runWithOptions(opts runOptions) int {
	rawArgs := opts.args
	if rawArgs == nil {
		rawArgs = os.Args[1:]
	}
	out := opts.out
	if out == nil {
		out = os.Stdout
	}
	ctx := opts.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	loadDotenv := opts.loadDotenv
	if loadDotenv == nil {
		loadDotenv = func() error { return godotenv.Load() }
	}
	newFetcher := opts.newFetcher
	if newFetcher == nil {
		newFetcher = func(token string) musixmatch.Fetcher { return musixmatch.NewClient(token) }
	}
	newWriter := opts.newWriter
	if newWriter == nil {
		newWriter = func() lyrics.Writer { return lyrics.NewLRCWriter() }
	}
	newApp := opts.newApp
	if newApp == nil {
		newApp = func(fetcher musixmatch.Fetcher, writer lyrics.Writer, inputs *queue.InputsQueue, cooldown int, mode string) appRunner {
			return app.NewApp(fetcher, writer, inputs, cooldown, mode)
		}
	}

	var args Args
	var legacy LegacyArgs
	parseTarget := any(&args)
	if !usesSubcommand(rawArgs) {
		parseTarget = &legacy
	}
	parser, err := arg.NewParser(arg.Config{Program: "mxlrcgo-svc", Out: out}, parseTarget)
	if err != nil {
		_, _ = fmt.Fprintln(out, err)
		return 2
	}
	if err := parser.Parse(rawArgs); err != nil {
		if err == arg.ErrHelp {
			if err := parser.WriteHelpForSubcommand(out); err != nil {
				_, _ = fmt.Fprintln(out, err)
				return 2
			}
			return 0
		}
		if usageErr := parser.WriteUsageForSubcommand(out); usageErr != nil {
			_, _ = fmt.Fprintln(out, usageErr)
			return 2
		}
		_, _ = fmt.Fprintln(out, err)
		return 2
	}

	_ = loadDotenv()

	switch {
	case !usesSubcommand(rawArgs):
		if legacy.Serve {
			return runServe(ctx, legacyServe(legacy), newFetcher, newWriter)
		}
		return runFetch(ctx, out, legacyFetch(legacy), newFetcher, newWriter, newApp)
	case args.Fetch != nil:
		return runFetch(ctx, out, *args.Fetch, newFetcher, newWriter, newApp)
	case args.Serve != nil:
		return runServe(ctx, *args.Serve, newFetcher, newWriter)
	case args.Scan != nil:
		return runScan(ctx, *args.Scan)
	case args.Library != nil:
		return runLibrary(ctx, out, *args.Library)
	case args.Keys != nil:
		return runKeys(ctx, out, *args.Keys)
	case args.Config != nil:
		return runConfig(out, *args.Config)
	default:
		_, _ = fmt.Fprintln(out, "missing subcommand")
		return 2
	}
}

func usesSubcommand(rawArgs []string) bool {
	if len(rawArgs) == 0 {
		return false
	}
	if rawArgs[0] == "-h" || rawArgs[0] == "--help" {
		return true
	}
	commands := map[string]bool{
		"fetch": true, "serve": true, "scan": true, "library": true, "keys": true, "config": true,
	}
	return commands[rawArgs[0]]
}

func legacyFetch(args LegacyArgs) FetchCmd {
	return FetchCmd{
		Song:       args.Song,
		Outdir:     args.Outdir,
		Cooldown:   args.Cooldown,
		Depth:      args.Depth,
		Update:     args.Update,
		Upgrade:    args.Upgrade,
		BFS:        args.BFS,
		Token:      args.Token,
		ConfigPath: args.ConfigPath,
	}
}

func legacyServe(args LegacyArgs) ServeCmd {
	return ServeCmd{
		Listen:     args.Listen,
		Outdir:     args.Outdir,
		Token:      args.Token,
		ConfigPath: args.ConfigPath,
		Depth:      args.Depth,
		Update:     args.Update,
		Upgrade:    args.Upgrade,
		BFS:        args.BFS,
	}
}

func runFetch(ctx context.Context, out io.Writer, args FetchCmd, newFetcher func(string) musixmatch.Fetcher, newWriter func() lyrics.Writer, newApp func(musixmatch.Fetcher, lyrics.Writer, *queue.InputsQueue, int, string) appRunner) int {
	if len(args.Song) == 0 {
		_, _ = fmt.Fprintln(out, "missing required positional argument: Song")
		return 2
	}
	cfg, err := config.Load(args.ConfigPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		return 1
	}
	token := args.Token
	if token == "" {
		token = cfg.API.Token
	}
	if token == "" {
		slog.Error("no API token provided: use --token flag, MUSIXMATCH_TOKEN env var, MXLRC_API_TOKEN env var, or config file")
		return 1
	}
	cooldown := cfg.API.Cooldown
	if args.Cooldown != nil {
		cooldown = *args.Cooldown
	}
	outdir := cfg.Output.Dir
	if args.Outdir != nil {
		outdir = *args.Outdir
	}

	inputs := queue.NewInputsQueue()
	sc := scanner.NewScanner()
	mode, err := sc.ParseInput(args.Song, outdir, args.Update, args.Upgrade, args.Depth, args.BFS, inputs)
	if err != nil {
		slog.Error("failed to parse input", "error", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "\n%d lyrics to fetch\n\n", inputs.Len())
	if mode != "dir" {
		if err := os.MkdirAll(outdir, 0750); err != nil { //nolint:gosec // user-specified output directory
			slog.Error("failed to create output directory", "error", err)
			return 1
		}
	}

	application := newApp(newFetcher(token), newWriter(), inputs, cooldown, mode)
	if err := application.Run(ctx); err != nil {
		slog.Error("application error", "error", err)
		return 1
	}
	return 0
}

func runServe(ctx context.Context, args ServeCmd, newFetcher func(string) musixmatch.Fetcher, newWriter func() lyrics.Writer) int {
	cfg, err := config.Load(args.ConfigPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		return 1
	}
	token := args.Token
	if token == "" {
		token = cfg.API.Token
	}
	if token == "" {
		slog.Error("no API token provided: serve needs a token for the worker")
		return 1
	}
	outdir := cfg.Output.Dir
	if args.Outdir != nil {
		outdir = *args.Outdir
	}
	sqlDB, err := db.Open(ctx, cfg.DB.Path)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		return 1
	}
	defer sqlDB.Close() //nolint:errcheck // best-effort close on shutdown

	authSvc, err := keyService(ctx, sqlDB, cfg.Server.WebhookAPIKeys)
	if err != nil {
		slog.Error("failed to configure authentication", "error", err)
		return 1
	}
	workQ := queue.NewDBQueue(sqlDB)
	w := worker.New(workQ, cache.New(sqlDB), newFetcher(token), newWriter())
	go runWorkerLoop(ctx, w, time.Duration(args.WorkInterval)*time.Second)
	go runScheduler(ctx, sqlDB, args)

	addr := cfg.Server.Addr
	if args.Listen != nil {
		addr = *args.Listen
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           server.NewHandler(authSvc, workQ, outdir),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Warn("HTTP server shutdown failed", "error", err)
		}
	}()
	slog.Info("starting HTTP server", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("HTTP server failed", "error", err)
		return 1
	}
	return 0
}

func runWorkerLoop(ctx context.Context, w *worker.Worker, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := w.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("worker run failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func runScheduler(ctx context.Context, sqlDB *sql.DB, args ServeCmd) {
	interval := defaultScanInterval
	if args.ScanInterval >= 0 {
		interval = time.Duration(args.ScanInterval) * time.Second
	}
	s := scheduler(sqlDB, scanner.ScanOptions{
		Update:   args.Update,
		Upgrade:  args.Upgrade,
		MaxDepth: args.Depth,
		BFS:      args.BFS,
	})
	s.Interval = interval
	if err := s.Run(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		slog.Warn("scheduler failed", "error", err)
	}
}

func runScan(ctx context.Context, args ScanCmd) int {
	cfg, err := config.Load(args.ConfigPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		return 1
	}
	sqlDB, err := db.Open(ctx, cfg.DB.Path)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		return 1
	}
	defer sqlDB.Close() //nolint:errcheck // best-effort close on shutdown

	s := scheduler(sqlDB, scanner.ScanOptions{
		Update:   args.Update,
		Upgrade:  args.Upgrade,
		MaxDepth: args.Depth,
		BFS:      args.BFS,
	})
	if err := s.RunOnce(ctx); err != nil {
		slog.Error("scan failed", "error", err)
		return 1
	}
	return 0
}

func scheduler(sqlDB *sql.DB, opts scanner.ScanOptions) scan.Scheduler {
	results := scan.New(sqlDB)
	enq := scan.Enqueuer{
		Results: results,
		Cache:   cache.New(sqlDB),
		Queue:   queue.NewDBQueue(sqlDB),
	}
	return scan.Scheduler{
		Libraries: library.New(sqlDB),
		Results:   results,
		Scanner:   scanner.NewScanner(),
		Options:   opts,
		OnScanComplete: func(ctx context.Context, lib models.Library, _ []models.ScanResult) error {
			return enq.EnqueuePending(ctx, lib.ID)
		},
	}
}

func runLibrary(ctx context.Context, out io.Writer, args LibraryCmd) int {
	path := libraryConfigPath(args)
	cfg, err := config.Load(path)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		return 1
	}
	sqlDB, err := db.Open(ctx, cfg.DB.Path)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		return 1
	}
	defer sqlDB.Close() //nolint:errcheck // best-effort close on shutdown

	repo := library.New(sqlDB)
	switch {
	case args.Add != nil:
		name := args.Add.Name
		if name == "" {
			name = filepath.Base(filepath.Clean(args.Add.Path))
		}
		lib, err := repo.Add(ctx, args.Add.Path, name)
		if err != nil {
			slog.Error("failed to add library", "error", err)
			return 1
		}
		_, _ = fmt.Fprintf(out, "%d\t%s\t%s\n", lib.ID, lib.Name, lib.Path)
	case args.List != nil:
		libs, err := repo.List(ctx)
		if err != nil {
			slog.Error("failed to list libraries", "error", err)
			return 1
		}
		for _, v := range libs {
			_, _ = fmt.Fprintf(out, "%d\t%s\t%s\n", v.ID, v.Name, v.Path)
		}
	case args.Remove != nil:
		if err := repo.Remove(ctx, args.Remove.ID); err != nil {
			slog.Error("failed to remove library", "error", err)
			return 1
		}
		_, _ = fmt.Fprintf(out, "removed library %d\n", args.Remove.ID)
	default:
		_, _ = fmt.Fprintln(out, "missing library subcommand")
		return 2
	}
	return 0
}

func libraryConfigPath(args LibraryCmd) string {
	switch {
	case args.Add != nil:
		return args.Add.ConfigPath
	case args.List != nil:
		return args.List.ConfigPath
	case args.Remove != nil:
		return args.Remove.ConfigPath
	default:
		return ""
	}
}

func runKeys(ctx context.Context, out io.Writer, args KeysCmd) int {
	path := keysConfigPath(args)
	cfg, err := config.Load(path)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		return 1
	}
	sqlDB, err := db.Open(ctx, cfg.DB.Path)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		return 1
	}
	defer sqlDB.Close() //nolint:errcheck // best-effort close on shutdown

	svc := auth.NewService(auth.NewSQLStore(sqlDB))
	switch {
	case args.Create != nil:
		scopes, err := parseScopes(args.Create.Scopes)
		if err != nil {
			slog.Error("failed to create API key", "error", err)
			return 1
		}
		created, err := svc.CreateKey(ctx, args.Create.Name, scopes)
		if err != nil {
			slog.Error("failed to create API key", "error", err)
			return 1
		}
		_, _ = fmt.Fprintf(out, "%s\n", created.Raw)
	case args.List != nil:
		keys, err := svc.ListKeys(ctx)
		if err != nil {
			slog.Error("failed to list API keys", "error", err)
			return 1
		}
		for _, v := range keys {
			revoked := ""
			if v.RevokedAt != nil {
				revoked = v.RevokedAt.Format(time.RFC3339)
			}
			_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", v.ID, v.Name, encodeScopes(v.Scopes), revoked)
		}
	case args.Revoke != nil:
		key, err := svc.RevokeKey(ctx, args.Revoke.Key)
		if err != nil {
			slog.Error("failed to revoke API key", "error", err)
			return 1
		}
		_, _ = fmt.Fprintf(out, "revoked %s\n", key.ID)
	default:
		_, _ = fmt.Fprintln(out, "missing keys subcommand")
		return 2
	}
	return 0
}

func keysConfigPath(args KeysCmd) string {
	switch {
	case args.Create != nil:
		return args.Create.ConfigPath
	case args.List != nil:
		return args.List.ConfigPath
	case args.Revoke != nil:
		return args.Revoke.ConfigPath
	default:
		return ""
	}
}

func parseScopes(values []string) ([]auth.Scope, error) {
	if len(values) == 0 {
		values = []string{string(auth.ScopeWebhook)}
	}
	scopes := make([]auth.Scope, 0, len(values))
	for _, v := range values {
		scopes = append(scopes, auth.Scope(v))
	}
	return auth.NormalizeScopes(scopes)
}

func keyService(ctx context.Context, sqlDB *sql.DB, rawKeys []string) (*auth.Service, error) {
	store := auth.NewSQLStore(sqlDB)
	for i, raw := range rawKeys {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if !strings.HasPrefix(raw, auth.KeyPrefix) {
			return nil, fmt.Errorf("webhook API key %d: invalid format", i+1)
		}
		hash, err := auth.HashKey(raw)
		if err != nil {
			return nil, err
		}
		if _, err := store.FindByHash(ctx, hash); err == nil {
			continue
		} else if !errors.Is(err, auth.ErrInvalidKey) {
			return nil, err
		}
		if len(hash) < 16 {
			return nil, fmt.Errorf("webhook API key %d: derived hash is too short", i+1)
		}
		if err := store.Create(ctx, auth.Key{
			ID:        hash[:16],
			Name:      fmt.Sprintf("webhook-%d", i+1),
			Hash:      hash,
			Scopes:    []auth.Scope{auth.ScopeWebhook},
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			return nil, err
		}
	}
	return auth.NewService(store), nil
}

func runConfig(out io.Writer, args ConfigCmd) int {
	path := configCommandPath(args)
	cfg, err := config.Load(path)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		return 1
	}
	switch {
	case args.Get != nil:
		v, ok := configValue(cfg, args.Get.Key)
		if !ok {
			_, _ = fmt.Fprintf(out, "unknown config key: %s\n", args.Get.Key)
			return 2
		}
		_, _ = fmt.Fprintln(out, v)
	case args.List != nil:
		for _, k := range configKeys() {
			v, _ := configValue(cfg, k)
			_, _ = fmt.Fprintf(out, "%s=%s\n", k, v)
		}
	case args.Set != nil:
		if err := setConfigValue(&cfg, args.Set.Key, args.Set.Value); err != nil {
			_, _ = fmt.Fprintln(out, err)
			return 2
		}
		if path == "" {
			path = defaultConfigPath()
		}
		if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
			slog.Error("failed to create config directory", "error", err)
			return 1
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600) //nolint:gosec // user-selected config path
		if err != nil {
			slog.Error("failed to open config file", "error", err)
			return 1
		}
		if err := toml.NewEncoder(f).Encode(cfg); err != nil {
			_ = f.Close()
			slog.Error("failed to write config", "error", err)
			return 1
		}
		if err := f.Close(); err != nil {
			slog.Error("failed to close config", "error", err)
			return 1
		}
		_, _ = fmt.Fprintf(out, "%s=%s\n", args.Set.Key, args.Set.Value)
	default:
		_, _ = fmt.Fprintln(out, "missing config subcommand")
		return 2
	}
	return 0
}

func configCommandPath(args ConfigCmd) string {
	switch {
	case args.Get != nil:
		return args.Get.ConfigPath
	case args.Set != nil:
		return args.Set.ConfigPath
	case args.List != nil:
		return args.List.ConfigPath
	default:
		return ""
	}
}

func configKeys() []string {
	return []string{
		"api.token",
		"api.cooldown",
		"output.dir",
		"db.path",
		"server.addr",
		"server.webhook_api_keys",
	}
}

func configValue(cfg config.Config, key string) (string, bool) {
	switch key {
	case "api.token":
		return cfg.API.Token, true
	case "api.cooldown":
		return strconv.Itoa(cfg.API.Cooldown), true
	case "output.dir":
		return cfg.Output.Dir, true
	case "db.path":
		return cfg.DB.Path, true
	case "server.addr":
		return cfg.Server.Addr, true
	case "server.webhook_api_keys":
		return strings.Join(cfg.Server.WebhookAPIKeys, ","), true
	default:
		return "", false
	}
}

func setConfigValue(cfg *config.Config, key string, value string) error {
	switch key {
	case "api.token":
		cfg.API.Token = value
	case "api.cooldown":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return fmt.Errorf("api.cooldown must be a non-negative integer")
		}
		cfg.API.Cooldown = n
	case "output.dir":
		cfg.Output.Dir = value
	case "db.path":
		cfg.DB.Path = value
	case "server.addr":
		cfg.Server.Addr = value
	case "server.webhook_api_keys":
		cfg.Server.WebhookAPIKeys = splitCSV(value)
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
	return nil
}

func defaultConfigPath() string {
	if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
		return filepath.Join(base, "mxlrcgo-svc", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("config.toml")
	}
	return filepath.Join(home, ".config", "mxlrcgo-svc", "config.toml")
}

func splitCSV(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func encodeScopes(scopes []auth.Scope) string {
	parts := make([]string, 0, len(scopes))
	for _, v := range scopes {
		parts = append(parts, string(v))
	}
	slices.Sort(parts)
	return strings.Join(parts, ",")
}
