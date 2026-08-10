package main

import (
	"context"
	"log/slog"

	"github.com/smlx/gogpack/internal/convert"
)

// ConvertCmd represents the `convert` command.
type ConvertCmd struct {
	BaseGame          string   `kong:"arg,required,name='base-game',help='Path to the base game .sh installer.'"`
	DLCs              []string `kong:"arg,optional,name='dlc',help='Paths to any DLC .sh installers.'"`
	ExitOnError       bool     `kong:"name='exit-on-error',help='Exit immediately on error without pausing to inspect the workspace.'"`
	PreserveWorkspace bool     `kong:"name='preserve-workspace',help='Do not clean up the workspace after building the flatpak.'"`
	Gamescope         bool     `kong:"name='gamescope',default='true',help='Enable Gamescope wrapper for the game (can be disabled with --gamescope=false).'"`
	RuntimeVersion    string   `kong:"name='runtime-version',default='25.08',help='The Flatpak SDK runtime version to use.'"`
}

// Run the convert command.
func (c *ConvertCmd) Run(ctx context.Context) error {
	return convert.NewConverter(slog.Default(), c.BaseGame, c.DLCs, !c.ExitOnError, c.PreserveWorkspace, c.Gamescope, c.RuntimeVersion).Run(ctx)
}
