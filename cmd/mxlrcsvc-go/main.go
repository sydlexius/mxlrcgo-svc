package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	arg "github.com/alexflint/go-arg"
	"github.com/joho/godotenv"
	"github.com/sydlexius/mxlrcsvc-go/internal/app"
	"github.com/sydlexius/mxlrcsvc-go/internal/config"
	"github.com/sydlexius/mxlrcsvc-go/internal/db"
	"github.com/sydlexius/mxlrcsvc-go/internal/lyrics"
	"github.com/sydlexius/mxlrcsvc-go/internal/musixmatch"
	"github.com/sydlexius/mxlrcsvc-go/internal/scanner"
)

// Args defines the CLI arguments for the application.
type Args struct {
	Song       []string `arg:"positional,required" help:"song information in [ artist,title ] format (required)"`
	Outdir     *string  `arg:"-o,--outdir" help:"output directory (default: from config or 'lyrics')"`
	Cooldown   *int     `arg:"-c,--cooldown" help:"cooldown time in seconds (default: from config or 15)"`
	Depth      int      `arg:"-d,--depth" help:"(directory mode) maximum recursion depth" default:"100"`
	Update     bool     `arg:"-u,--update" help:"(directory mode) re-fetch and overwrite existing .lrc files"`
	Upgrade    bool     `arg:"--upgrade" help:"(directory mode) re-fetch songs with .txt (unsynced) to promote to .lrc if synced lyrics are now available; implied by --update"`
	BFS        bool     `arg:"--bfs" help:"(directory mode) use breadth-first-search traversal"`
	Token      string   `arg:"-t,--token" help:"musixmatch token (or MUSIXMATCH_TOKEN / MXLRC_API_TOKEN env var, or config file)" default:""`
	ConfigPath string   `arg:"--config" help:"path to config file (default: XDG)" default:""`
}

func main() {
	os.Exit(run())
}

// run executes the application and returns an exit code.
// Using a helper function ensures deferred cleanup (e.g. sqlDB.Close) runs
// before os.Exit is called, while still producing a non-zero exit on error.
func run() int {
	var args Args
	arg.MustParse(&args)

	// Load .env file if present (does NOT overwrite existing env vars).
	// Error is ignored -- .env file is optional.
	_ = godotenv.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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

	// Token precedence: CLI flag > env vars (handled in config.Load) > config file.
	token := args.Token
	if token == "" {
		token = cfg.API.Token
	}
	if token == "" {
		slog.Error("no API token provided: use --token flag, MUSIXMATCH_TOKEN env var, MXLRC_API_TOKEN env var, or config file")
		return 1
	}

	// Cooldown: explicit CLI flag wins; otherwise use config (which has its own default).
	cooldown := cfg.API.Cooldown
	if args.Cooldown != nil {
		cooldown = *args.Cooldown
	}

	// Outdir: explicit CLI flag wins; otherwise use config (which has its own default).
	outdir := cfg.Output.Dir
	if args.Outdir != nil {
		outdir = *args.Outdir
	}

	inputs := app.NewInputsQueue()
	sc := scanner.NewScanner()
	mode, err := sc.ParseInput(args.Song, outdir, args.Update, args.Upgrade, args.Depth, args.BFS, inputs)
	if err != nil {
		slog.Error("failed to parse input", "error", err)
		return 1
	}
	fmt.Printf("\n%d lyrics to fetch\n\n", inputs.Len())

	if mode != "dir" {
		if err := os.MkdirAll(outdir, 0750); err != nil { //nolint:gosec // user-specified output directory
			slog.Error("failed to create output directory", "error", err)
			return 1
		}
	}

	mx := musixmatch.NewClient(token)
	w := lyrics.NewLRCWriter()
	application := app.NewApp(mx, w, inputs, cooldown, mode)

	if err := application.Run(ctx); err != nil {
		slog.Error("application error", "error", err)
		return 1
	}
	return 0
}
