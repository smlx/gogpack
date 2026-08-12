// Package main implements the command-line interface of a server.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"
)

// CLI represents the command-line interface.
type CLI struct {
	Debug   bool       `kong:"name='debug',help='Enable debug level logging.'"`
	Version VersionCmd `kong:"cmd,help='Print version information'"`
	Convert ConvertCmd `kong:"cmd,help='Convert a GOG Linux installer to a Flatpak'"`
}

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// parse CLI config
	cli := CLI{}
	kctx := kong.Parse(&cli,
		kong.UsageOnError(),
		kong.BindFor(ctx),
	)

	// configure logger
	logLevel := slog.LevelInfo
	if cli.Debug {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	})))

	// execute CLI
	kctx.FatalIfErrorf(kctx.Run())
}
