package convert

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"mvdan.cc/sh/v3/syntax"
)

const launcherTemplate = `#!/bin/sh
if [ -n "$FLATPAK_DEBUG" ]; then
    set -x
fi
export LC_ALL=C
{{if .LDLibraryPath}}
export LD_LIBRARY_PATH="{{.LDLibraryPath}}:$LD_LIBRARY_PATH"
{{end}}
{{if and .HasWwise .SDLPath}}
export LD_PRELOAD="{{.SDLPath}}:$LD_PRELOAD"
{{end}}

cd "$(dirname "/app/bin/{{.MainExe}}")" || exit 1
{{if .EnableGamescope}}
if [ ! -x "/usr/lib/extensions/vulkan/gamescope/bin/gamescope" ]; then
    echo "Error: Gamescope extension not found." >&2
    exit 1
fi
export PATH="/usr/lib/extensions/vulkan/gamescope/bin:$PATH"
trap 'kill -TERM -0 2>/dev/null || true' EXIT
gamescope --force-grab-cursor -f $GAMESCOPE_ARGS -- "./$(basename "{{.MainExe}}")" "$@"
{{else}}
exec "./$(basename "{{.MainExe}}")" "$@"
{{end}}
`

func extractLDLibraryPaths(startShPath string) []string {
	var ldLibraryPaths []string
	f, err := os.Open(startShPath)
	if err != nil {
		return ldLibraryPaths
	}
	defer f.Close()

	p := syntax.NewParser()
	fNode, err := p.Parse(f, "")
	if err != nil {
		return ldLibraryPaths
	}

	syntax.Walk(fNode, func(node syntax.Node) bool {
		stmt, ok := node.(*syntax.Stmt)
		if !ok {
			return true
		}
		cmd, ok := stmt.Cmd.(*syntax.DeclClause)
		if !ok {
			return true
		}
		for _, assign := range cmd.Args {
			if assign.Name == nil || assign.Name.Value != "LD_LIBRARY_PATH" || assign.Value == nil {
				continue
			}
			for _, wordPart := range assign.Value.Parts {
				lit, ok := wordPart.(*syntax.Lit)
				if !ok {
					continue
				}
				paths := strings.SplitSeq(lit.Value, ":")
				for p := range paths {
					if p == "" || p == "$LD_LIBRARY_PATH" || p == "${LD_LIBRARY_PATH}" {
						continue
					}
					p = strings.TrimPrefix(p, "./")
					ldLibraryPaths = append(ldLibraryPaths, "/app/bin/"+p)
				}
			}
		}
		return true
	})

	return ldLibraryPaths
}

// generateWrapper parses start.sh and generates a new launcher.sh.
func (c *Converter) generateWrapper() error {
	startShPath := filepath.Join(c.workspace, "start.sh")
	ldLibraryPaths := extractLDLibraryPaths(startShPath)

	// Check for Wwise libAkSoundEngine.so
	var hasWwise bool
	var sdlPath string

	err := filepath.WalkDir(c.workspace, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			if d.Name() == LibAkSoundEngine {
				hasWwise = true
			} else if d.Name() == LibSDL2 || d.Name() == LibSDL2_2_0 {
				rel, _ := filepath.Rel(c.workspace, filepath.Dir(path))
				sdlPath = "/app/bin/" + rel + "/" + d.Name()
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("couldn't walk workspace for dependencies: %w", err)
	}

	tmpl, err := template.New("launcher").Parse(launcherTemplate)
	if err != nil {
		return fmt.Errorf("couldn't parse launcher template: %w", err)
	}

	launcherPath := filepath.Join(c.workspace, "launcher.sh")
	f, err := os.OpenFile(launcherPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("couldn't create launcher.sh: %w", err)
	}
	defer f.Close()

	data := struct {
		LDLibraryPath   string
		HasWwise        bool
		SDLPath         string
		MainExe         string
		EnableGamescope bool
	}{
		LDLibraryPath:   strings.Join(ldLibraryPaths, ":"),
		HasWwise:        hasWwise,
		SDLPath:         sdlPath,
		MainExe:         c.mainExe,
		EnableGamescope: c.enableGamescope,
	}

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("couldn't execute launcher template: %w", err)
	}

	return nil
}
