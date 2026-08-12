package convert

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// fixEmulatorConfigs translates backslashes to forward slashes and remaps cloud_saves.
func (c *Converter) fixEmulatorConfigs() error {
	supportDir := filepath.Join(c.workspace, "__support", "app")

	// Ensure the directory exists before walking
	if _, err := os.Stat(supportDir); os.IsNotExist(err) {
		return nil // No emulator config found, this is fine
	}

	return filepath.WalkDir(supportDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".conf") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("couldn't read %s: %w", path, err)
		}

		// Translate backslashes to forward slashes
		modified := strings.ReplaceAll(string(content), "\\", "/")

		// Remap cloud_saves mounts
		cloudSavesRegexForward := regexp.MustCompile(`(?i)(mount\s+[a-z]\s+)"?.*?/cloud_saves"?`)
		replacement := fmt.Sprintf(`$1"~/.var/app/com.gog.%s/cloud_saves"`, sanitizeAppIDName(c.meta.Name))
		modified = cloudSavesRegexForward.ReplaceAllString(modified, replacement)

		if err := os.WriteFile(path, []byte(modified), 0644); err != nil {
			return fmt.Errorf("couldn't write %s: %w", path, err)
		}

		return nil
	})
}
