package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/alexflint/go-arg"
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
	Update   bool     `arg:"-u,--update" help:"(directory mode) update existing lyrics file"`
	BFS      bool     `arg:"--bfs" help:"(directory mode) use breadth-first-search traversal"`
	Token    string   `arg:"-t,--token" help:"musixmatch token" default:""`
}

func main() {
	var args Args
	arg.MustParse(&args)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	inputs := app.NewInputsQueue()
	sc := scanner.NewScanner()
	mode, err := sc.ParseInput(args.Song, args.Outdir, args.Update, args.Depth, args.BFS, inputs)
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

	var token string
	if token = args.Token; args.Token == "" {
		token = "2203269256ff7abcb649269df00e14c833dbf4ddfb5b36a1aae8b0"
	}

	mx := musixmatch.NewClient(token)
	w := lyrics.NewLRCWriter()
	application := app.NewApp(mx, w, inputs, args.Cooldown, mode)

	if err := application.Run(ctx); err != nil {
		slog.Error("application error", "error", err)
		os.Exit(1)
	}
}
