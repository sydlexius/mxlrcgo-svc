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
	"github.com/sydlexius/mxlrcsvc-go/internal/lyrics"
	"github.com/sydlexius/mxlrcsvc-go/internal/musixmatch"
	"github.com/sydlexius/mxlrcsvc-go/internal/scanner"
)

// Args defines the CLI arguments for the application.
type Args struct {
	Song     []string `arg:"positional,required" help:"song information in [ artist,title ] format (required)"`
	Outdir   string   `arg:"-o,--outdir" help:"output directory" default:"lyrics"`
	Cooldown int      `arg:"-c,--cooldown" help:"cooldown time in seconds" default:"15"`
	Depth    int      `arg:"-d,--depth" help:"(directory mode) maximum recursion depth" default:"100"`
	Update   bool     `arg:"-u,--update" help:"(directory mode) re-fetch and overwrite existing .lrc files"`
	Upgrade  bool     `arg:"--upgrade" help:"(directory mode) re-fetch songs with .txt (unsynced) to promote to .lrc if synced lyrics are now available; implied by --update"`
	BFS      bool     `arg:"--bfs" help:"(directory mode) use breadth-first-search traversal"`
	Token    string   `arg:"-t,--token" help:"musixmatch token (or set MUSIXMATCH_TOKEN env var / .env file)" default:""`
}

func main() {
	var args Args
	arg.MustParse(&args)

	// Load .env file if present (does NOT overwrite existing env vars).
	// Error is ignored -- .env file is optional.
	_ = godotenv.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	inputs := app.NewInputsQueue()
	sc := scanner.NewScanner()
	mode, err := sc.ParseInput(args.Song, args.Outdir, args.Update, args.Upgrade, args.Depth, args.BFS, inputs)
	if err != nil {
		slog.Error("failed to parse input", "error", err)
		os.Exit(1)
	}
	fmt.Printf("\n%d lyrics to fetch\n\n", inputs.Len())

	if mode == "dir" {
		args.Outdir = ""
	} else {
		if err := os.MkdirAll(args.Outdir, 0750); err != nil { //nolint:gosec // user-specified output directory
			slog.Error("failed to create output directory", "error", err)
			os.Exit(1)
		}
	}

	token := args.Token
	if token == "" {
		token = os.Getenv("MUSIXMATCH_TOKEN")
	}
	if token == "" {
		slog.Error("no API token provided: use --token flag, MUSIXMATCH_TOKEN env var, or .env file")
		os.Exit(1)
	}

	mx := musixmatch.NewClient(token)
	w := lyrics.NewLRCWriter()
	application := app.NewApp(mx, w, inputs, args.Cooldown, mode)

	if err := application.Run(ctx); err != nil {
		slog.Error("application error", "error", err)
		os.Exit(1)
	}
}
