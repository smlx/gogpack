package convert

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

func sanitizeAppIDName(name string) string {
	var sb strings.Builder
	capitalizeNext := true
	for _, r := range name {
		if unicode.IsSpace(r) || r == '-' || r == '_' {
			capitalizeNext = true
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if capitalizeNext {
				sb.WriteRune(unicode.ToUpper(r))
				capitalizeNext = false
			} else {
				sb.WriteRune(r)
			}
		}
	}
	return sb.String()
}

// executeBuild runs flatpak-builder and packages the result.
func (c *Converter) executeBuild(ctx context.Context) error {
	manifestPath := filepath.Join(c.workspace, "manifest.yaml")
	buildDir := filepath.Join(c.workspace, "build-dir")
	exportDir := filepath.Join(c.workspace, "export-dir")
	stateDir := filepath.Join(c.workspace, ".flatpak-builder")
	appID := fmt.Sprintf("com.gog.%s", sanitizeAppIDName(c.meta.Name))
	outputBundle := fmt.Sprintf("%s.flatpak", appID)

	c.log.Info("Running flatpak-builder...")
	cmd := exec.CommandContext(ctx, "flatpak-builder", "--force-clean", "--repo="+exportDir, "--state-dir="+stateDir, "--disable-download", "--disable-rofiles-fuse", buildDir, manifestPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("flatpak-builder failed: %w: %s", err, out)
	}

	c.log.Info("Packaging Flatpak bundle...")
	cmd = exec.CommandContext(ctx, "flatpak", "build-bundle", exportDir, outputBundle, appID)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("flatpak build-bundle failed: %w: %s", err, out)
	}

	c.log.Info("Successfully created Flatpak", "bundle", outputBundle)
	return nil
}
