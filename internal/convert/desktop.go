package convert

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"text/template"
	"time"
)

const desktopTemplate = `[Desktop Entry]
Name={{.GameName}}
Exec=launcher.sh
Icon={{.AppID}}
Terminal=false
Type=Application
Categories=Game;
`

const appstreamTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<component type="desktop-application">
  <id>{{.AppID}}</id>
  <name>{{.GameName}}</name>
  <summary>{{.GameName}}</summary>
  <metadata_license>CC0-1.0</metadata_license>
  <project_license>LicenseRef-proprietary</project_license>
  <developer id="com.gog">
    <name>GOG.com</name>
  </developer>
  <description>
    <p>This is the GOG.com game {{.GameName}} packaged for Flatpak on Linux using gogpack.</p>
  </description>
  <url type="homepage">https://www.gog.com</url>
  <launchable type="desktop-id">{{.AppID}}.desktop</launchable>
  <icon type="stock">{{.AppID}}</icon>
  <categories>
    <category>Game</category>
  </categories>
  <releases>
    <release version="{{.Version}}" date="{{.Date}}" />
  </releases>
</component>
`

var (
	parsedDesktopTemplate   = template.Must(template.New("desktop").Parse(desktopTemplate))
	parsedAppstreamTemplate = template.Must(template.New("appstream").Parse(appstreamTemplate))
)

// generateDesktopIntegration looks for support.ico or gog.ico, extracts the PNG,
// and creates a .desktop file.
func (c *Converter) generateDesktopIntegration() error {
	var iconPath string
	for _, name := range []string{"support.ico", "gog.ico"} {
		p := filepath.Join(c.workspace, name)
		if _, err := os.Stat(p); err == nil {
			iconPath = p
			break
		}
	}

	appID := fmt.Sprintf("com.gog.%s", sanitizeAppIDName(c.meta.Name))

	if iconPath != "" {
		pngPath := filepath.Join(c.workspace, appID+".png")
		if err := extractPNGFromICO(iconPath, pngPath); err != nil {
			c.log.Warn("couldn't extract PNG from ICO", "iconPath", iconPath, "error", err)
		}
	} else {
		pngPath := filepath.Join(c.workspace, appID+".png")
		// Fallback: look for a .png in the workspace (often provided by games like World of Goo)
		err := filepath.WalkDir(c.workspace, func(path string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && filepath.Ext(d.Name()) == ".png" {
				if data, err := os.ReadFile(path); err == nil {
					if err := os.WriteFile(pngPath, data, 0644); err != nil {
						c.log.Warn("couldn't write fallback PNG", "error", err)
					}
					return filepath.SkipAll // Stop at the first PNG
				}
			}
			return nil
		})
		if err != nil {
			c.log.Warn("couldn't walk workspace to find fallback PNG", "error", err)
		}
	}

	data := struct {
		GameName string
		AppID    string
		Version  string
		Date     string
	}{
		GameName: c.meta.Name,
		AppID:    appID,
		Version:  string(c.meta.Version),
		Date:     time.Now().Format("2006-01-02"),
	}

	var buf bytes.Buffer
	if err := parsedDesktopTemplate.Execute(&buf, data); err != nil {
		return fmt.Errorf("couldn't execute desktop template: %w", err)
	}

	desktopPath := filepath.Join(c.workspace, appID+".desktop")
	if err := os.WriteFile(desktopPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("couldn't write desktop file: %w", err)
	}

	buf.Reset()
	if err := parsedAppstreamTemplate.Execute(&buf, data); err != nil {
		return fmt.Errorf("couldn't execute appstream template: %w", err)
	}

	appstreamPath := filepath.Join(c.workspace, appID+".metainfo.xml")
	if err := os.WriteFile(appstreamPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("couldn't write appstream file: %w", err)
	}

	return nil
}

// extractPNGFromICO parses the ICO header and extracts the largest PNG image found.
func extractPNGFromICO(icoPath, outPath string) error {
	data, err := os.ReadFile(icoPath)
	if err != nil {
		return err
	}

	if len(data) < 6 {
		return fmt.Errorf("file too small to be ICO")
	}

	if binary.LittleEndian.Uint16(data[0:2]) != 0 || binary.LittleEndian.Uint16(data[2:4]) != 1 {
		return fmt.Errorf("invalid ICO header")
	}

	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if len(data) < 6+count*16 {
		return fmt.Errorf("ICO file truncated")
	}

	var bestImg image.Image
	var maxArea int

	for i := range count {
		offset := 6 + i*16
		width := int(data[offset])
		if width == 0 {
			width = 256
		}
		height := int(data[offset+1])
		if height == 0 {
			height = 256
		}

		size := int(binary.LittleEndian.Uint32(data[offset+8 : offset+12]))
		dataOffset := int(binary.LittleEndian.Uint32(data[offset+12 : offset+16]))

		if dataOffset+size > len(data) {
			continue
		}

		imgData := data[dataOffset : dataOffset+size]

		// Try to decode as PNG
		img, err := png.Decode(bytes.NewReader(imgData))
		if err == nil {
			area := width * height
			if area > maxArea {
				maxArea = area
				bestImg = img
			}
		}
	}

	if bestImg == nil {
		return fmt.Errorf("no valid PNG images found in ICO")
	}

	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	return png.Encode(out, bestImg)
}
