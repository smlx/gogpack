package convert

import (
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"
)

type Extension struct {
	Directory    string `json:"directory"`
	Version      string `json:"version"`
	AddLDPath    string `json:"add-ld-path,omitempty"`
	Autodownload *bool  `json:"autodownload,omitempty"`
	Autodelete   *bool  `json:"autodelete,omitempty"`
}

type Source struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

type Module struct {
	Name          string   `json:"name"`
	Buildsystem   string   `json:"buildsystem"`
	BuildCommands []string `json:"build-commands"`
	Sources       []Source `json:"sources"`
}

type Manifest struct {
	AppID          string               `json:"app-id"`
	Runtime        string               `json:"runtime"`
	RuntimeVersion string               `json:"runtime-version"`
	Sdk            string               `json:"sdk"`
	Command        string               `json:"command"`
	FinishArgs     []string             `json:"finish-args"`
	AddExtensions  map[string]Extension `json:"add-extensions,omitempty"`
	Modules        []Module             `json:"modules"`
}

// generateManifest synthesizes the manifest.yaml for flatpak-builder.
func (c *Converter) generateManifest() error {
	appID := fmt.Sprintf("com.gog.%s", sanitizeAppIDName(c.meta.Name))

	manifest := Manifest{
		AppID:          appID,
		Runtime:        "org.freedesktop.Platform",
		RuntimeVersion: c.runtimeVersion,
		Sdk:            "org.freedesktop.Sdk",
		Command:        "launcher.sh",
		FinishArgs: []string{
			"--share=ipc",
			"--socket=wayland",
			"--socket=pulseaudio",
			"--device=dri",
			"--persist=.",
		},
		Modules: []Module{
			{
				Name:        "game",
				Buildsystem: "simple",
				BuildCommands: []string{
					"mkdir -p /app/bin/ /app/share/applications/ /app/share/icons/hicolor/256x256/apps/ /app/share/metainfo/",
					"cp -a game /app/bin/",
					"cp launcher.sh /app/bin/",
					fmt.Sprintf("cp %s.desktop /app/share/applications/", appID),
					fmt.Sprintf("cp %s.png /app/share/icons/hicolor/256x256/apps/ 2>/dev/null || true", appID),
					fmt.Sprintf("cp %s.metainfo.xml /app/share/metainfo/", appID),
				},
				Sources: []Source{
					{
						Type: "dir",
						Path: c.workspace,
					},
				},
			},
		},
	}

	if c.is32Bit {
		manifest.AddExtensions = map[string]Extension{
			"org.freedesktop.Platform.Compat.i386": {
				Directory: "lib/i386-linux-gnu",
				Version:   c.runtimeVersion,
			},
			"org.freedesktop.Platform.GL32.default": {
				Directory: "lib/i386-linux-gnu/GL",
				Version:   c.runtimeVersion,
				AddLDPath: "lib",
			},
		}
	}

	if c.enableGamescope {
		manifest.Modules[0].BuildCommands = append(manifest.Modules[0].BuildCommands, "mkdir -p /app/extensions/vulkan/gamescope")
		if manifest.AddExtensions == nil {
			manifest.AddExtensions = make(map[string]Extension)
		}
		autodownload := true
		manifest.AddExtensions["org.freedesktop.Platform.VulkanLayer.gamescope"] = Extension{
			Directory:    "extensions/vulkan/gamescope",
			Version:      c.runtimeVersion,
			Autodownload: &autodownload,
		}
		manifest.FinishArgs = append(manifest.FinishArgs, "--socket=fallback-x11", "--filesystem=xdg-run/pipewire-0")
	} else {
		manifest.FinishArgs = append(manifest.FinishArgs, "--socket=x11")
	}

	data, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("couldn't marshal manifest: %w", err)
	}

	manifestPath := filepath.Join(c.workspace, ManifestName)
	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		return fmt.Errorf("couldn't write manifest: %w", err)
	}

	return nil
}
