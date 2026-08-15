package convert

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"golang.org/x/image/draw"
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

func (c *Converter) generateDesktopIntegration() error {
	var icoPath string
	for _, name := range []string{"support.ico", "gog.ico"} {
		p := filepath.Join(c.workspace, name)
		if _, err := os.Stat(p); err == nil {
			icoPath = p
			break
		}
	}

	appID := fmt.Sprintf("com.gog.%s", sanitizeAppIDName(c.meta.Name))
	pngPath := filepath.Join(c.workspace, appID+".png")

	if icoPath != "" {
		if err := extractPNGFromICO(icoPath, pngPath); err != nil {
			c.log.Warn("couldn't extract PNG from ICO", "iconPath", icoPath, "error", err)
		}
	} else {
		// Try support/icon.png first (common in newer GOG installers)
		supportIcon := filepath.Join(c.workspace, "support", "icon.png")
		if _, err := os.Stat(supportIcon); err == nil {
			if err := processPNGIcon(supportIcon, pngPath, c.log); err != nil {
				c.log.Warn("couldn't process support/icon.png", "error", err)
			}
		} else {
			// Fallback: search workspace for any .png file
			err := filepath.WalkDir(c.workspace, func(path string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() || filepath.Ext(d.Name()) != ".png" {
					return nil
				}
				if err = processPNGIcon(path, pngPath, c.log); err != nil {
					return nil // keep searching
				}
				return filepath.SkipAll // stop searching
			})
			if err != nil {
				c.log.Warn("couldn't walk workspace to find fallback PNG", "error", err)
			}
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
// processPNGIcon decodes a PNG, resizes it if needed, renames the original, and writes it to the target path.
func processPNGIcon(srcPath, dstPath string, log *slog.Logger) error {
	// open and decode image
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return err
	}
	// ensure image meets flatpak size limits (max 512x512)
	resized, wasResized := resizeImage(img, 512)
	if wasResized {
		// preserve original before resizing
		origPath := srcPath[:len(srcPath)-4] + ".orig.png"
		if err = os.Rename(srcPath, origPath); err != nil {
			log.Warn("couldn't rename original PNG", "error", err)
		}
	} else {
		// no resize needed: use image as-is
		if srcPath != dstPath {
			if err = os.Rename(srcPath, dstPath); err != nil {
				log.Warn("couldn't move original PNG to target path", "error", err)
			}
		}
		return nil
	}
	// write out png icon
	out, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer out.Close()
	if err := png.Encode(out, resized); err != nil {
		log.Warn("couldn't encode resized PNG", "error", err)
		return err
	}
	return nil
}

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
		// attempt decode
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

	bestImg, _ = resizeImage(bestImg, 512)

	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	return png.Encode(out, bestImg)
}

// resizeImage resizes an image if its dimensions exceed maxDim, preserving aspect ratio.
func resizeImage(img image.Image, maxDim int) (image.Image, bool) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= maxDim && h <= maxDim {
		return img, false
	}

	var newW, newH int
	if w > h {
		newW = maxDim
		newH = int(float64(h) * float64(maxDim) / float64(w))
	} else {
		newH = maxDim
		newW = int(float64(w) * float64(maxDim) / float64(h))
	}

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)
	return dst, true
}
