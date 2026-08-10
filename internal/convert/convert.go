package convert

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const (
	TargetPrefix     = "data/noarch/"
	LibAkSoundEngine = "libAkSoundEngine.so"
	LibSDL2          = "libSDL2.so"
	LibSDL2_2_0      = "libSDL2-2.0.so.0"
	ManifestName     = "manifest.yaml"
)

// Converter encapsulates the state for a single conversion job.
type Converter struct {
	log               *slog.Logger
	workspace         string
	baseGame          string
	dlcs              []string
	pauseOnError      bool
	preserveWorkspace bool
	enableGamescope   bool
	runtimeVersion    string
	meta              *GameMetadata
	mainExe           string
	is32Bit           bool
}

// NewConverter creates a new Converter.
func NewConverter(log *slog.Logger, baseGame string, dlcs []string, pauseOnError bool, preserveWorkspace bool, enableGamescope bool, runtimeVersion string) *Converter {
	return &Converter{
		log:               log,
		baseGame:          baseGame,
		dlcs:              dlcs,
		pauseOnError:      pauseOnError,
		preserveWorkspace: preserveWorkspace,
		enableGamescope:   enableGamescope,
		runtimeVersion:    runtimeVersion,
	}
}

func (c *Converter) Run(ctx context.Context) (err error) {
	c.log.Info("Converting base game", "baseGame", c.baseGame)

	workspace, err := os.MkdirTemp("", "gog-flatpak-build-*")
	if err != nil {
		return fmt.Errorf("couldn't create temp workspace: %v", err)
	}
	c.workspace = workspace
	defer func() {
		if err != nil && c.pauseOnError {
			fmt.Printf("\nError encountered: %v\n", err)
			fmt.Printf("Workspace preserved at: %s\n", c.workspace)
			fmt.Print("Press Enter to continue and clean up workspace...")
			bufio.NewReader(os.Stdin).ReadBytes('\n')
		}
		if !c.preserveWorkspace {
			os.RemoveAll(c.workspace)
		} else {
			c.log.Info("Workspace preserved at", "workspace", c.workspace)
		}
	}()
	c.log.Info("Created workspace", "workspace", c.workspace)

	if err := c.extractPayload(c.baseGame); err != nil {
		return fmt.Errorf("couldn't extract base game: %v", err)
	}

	for _, dlc := range c.dlcs {
		c.log.Info("Extracting DLC", "dlc", dlc)
		if err := c.extractPayload(dlc); err != nil {
			return fmt.Errorf("couldn't extract DLC %s: %v", dlc, err)
		}
	}

	c.log.Info("Applying executable permissions...")
	if err := c.fixPermissions(); err != nil {
		return fmt.Errorf("couldn't fix permissions: %v", err)
	}

	c.log.Info("Parsing game metadata...")
	meta, err := c.parseMetadata()
	if err != nil {
		return fmt.Errorf("couldn't parse metadata: %v", err)
	}
	c.meta = meta
	c.log.Info("Parsed metadata", "name", c.meta.Name, "clientId", c.meta.ClientID, "version", c.meta.Version)

	for _, task := range c.meta.PlayTasks {
		if task.Type == "FileTask" && task.Path != "" {
			c.mainExe = task.Path
			break
		}
	}

	if c.mainExe != "" {
		if _, err := os.Stat(filepath.Join(c.workspace, c.mainExe)); errors.Is(err, os.ErrNotExist) {
			if _, err := os.Stat(filepath.Join(c.workspace, "game", c.mainExe)); err == nil {
				c.mainExe = filepath.Join("game", c.mainExe)
			}
		}

		c.log.Info("Analyzing executable architecture", "executable", c.mainExe)
		is32Bit, err := c.checkArchitecture()
		if err != nil {
			return fmt.Errorf("couldn't check architecture: %v", err)
		}
		c.is32Bit = is32Bit
		if c.is32Bit {
			c.log.Info("Detected 32-bit executable.")
		}
	} else {
		c.log.Warn("Could not identify main executable from metadata.")
	}

	c.log.Info("Translating emulator configurations if present...")
	if err := c.fixEmulatorConfigs(); err != nil {
		return fmt.Errorf("couldn't fix emulator configurations: %v", err)
	}

	c.log.Info("Generating desktop integration...")
	if err := c.generateDesktopIntegration(); err != nil {
		return fmt.Errorf("couldn't generate desktop integration: %v", err)
	}

	c.log.Info("Synthesizing launch wrapper...")
	if err := c.generateWrapper(); err != nil {
		return fmt.Errorf("couldn't generate wrapper: %v", err)
	}

	c.log.Info("Synthesizing Flatpak manifest...")
	if err := c.generateManifest(); err != nil {
		return fmt.Errorf("couldn't generate manifest: %v", err)
	}

	c.log.Info("Executing Flatpak build...")
	if err := c.executeBuild(ctx); err != nil {
		return fmt.Errorf("couldn't execute build: %v", err)
	}

	return nil
}

func (c *Converter) extractPayload(installerPath string) error {
	r, err := zip.OpenReader(installerPath)
	if err != nil {
		return fmt.Errorf("couldn't open zip reader for %s: %w", installerPath, err)
	}
	defer r.Close()

	for _, f := range r.File {
		if !strings.HasPrefix(f.Name, TargetPrefix) {
			continue
		}

		relPath := strings.TrimPrefix(f.Name, TargetPrefix)
		if relPath == "" {
			continue
		}

		if !filepath.IsLocal(relPath) {
			return fmt.Errorf("invalid path in zip archive (potential zip slip): %s", f.Name)
		}

		outPath := filepath.Join(c.workspace, relPath)

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(outPath, 0755); err != nil {
				return fmt.Errorf("couldn't create directory %s: %w", outPath, err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return fmt.Errorf("couldn't create directory %s: %w", filepath.Dir(outPath), err)
		}

		if err := c.extractFile(f, outPath); err != nil {
			return fmt.Errorf("couldn't extract file %s: %w", outPath, err)
		}
	}

	return nil
}

func (c *Converter) extractFile(f *zip.File, outPath string) error {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("couldn't open zip file %s: %w", f.Name, err)
	}
	defer rc.Close()

	out, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode()|0200)
	if err != nil {
		return fmt.Errorf("couldn't create output file %s: %w", outPath, err)
	}

	if _, err = io.Copy(out, rc); err != nil {
		out.Close()
		return fmt.Errorf("couldn't copy contents to %s: %w", outPath, err)
	}

	if err := out.Close(); err != nil {
		return fmt.Errorf("couldn't close output file %s: %w", outPath, err)
	}
	return nil
}

func (c *Converter) fixPermissions() error {
	elfMagic := []byte{0x7F, 'E', 'L', 'F'}
	shebang := []byte{'#', '!'}

	return filepath.WalkDir(c.workspace, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("could not open %s: %w", path, err)
		}
		defer f.Close()

		header := make([]byte, 4)
		n, err := io.ReadFull(f, header)
		if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
			return fmt.Errorf("could not read header of %s: %w", path, err)
		}

		if n >= 4 && bytes.Equal(header[:4], elfMagic) || (n >= 2 && bytes.Equal(header[:2], shebang)) {
			// Needs executable permissions
			info, err := d.Info()
			if err != nil {
				return err
			}
			err = os.Chmod(path, info.Mode()|0111)
			if err != nil {
				return fmt.Errorf("couldn't chmod %s: %w", path, err)
			}
		}

		return nil
	})
}
