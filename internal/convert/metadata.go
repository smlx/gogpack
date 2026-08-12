package convert

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrMetadataNotFound is returned when no valid game metadata info file can be found.
var ErrMetadataNotFound = errors.New("metadata not found")

// FlexString is a string that unmarshals from either a JSON string or a JSON number.
type FlexString string

// UnmarshalJSON implements json.Unmarshaler.
func (fs *FlexString) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*fs = FlexString(s)
		return nil
	}
	*fs = FlexString(b)
	return nil
}

// GameMetadata represents the parsed data from goggame-*.info
type GameMetadata struct {
	Name      string     `json:"name"`
	Version   FlexString `json:"version"`
	ClientID  string     `json:"clientId"`
	IsDLC     bool       `json:"isDLC"`
	PlayTasks []struct {
		Path string `json:"path"`
		Type string `json:"type"`
	} `json:"playTasks"`
}

// parseMetadata finds and parses the base game info file.
func (c *Converter) parseMetadata() (*GameMetadata, error) {
	files, err := filepath.Glob(filepath.Join(c.workspace, "goggame-*.info"))
	if err != nil {
		return nil, fmt.Errorf("couldn't glob info files: %w", err)
	}

	gameFiles, err := filepath.Glob(filepath.Join(c.workspace, "game", "goggame-*.info"))
	if err != nil {
		return nil, fmt.Errorf("couldn't glob info files in game dir: %w", err)
	}
	files = append(files, gameFiles...)

	if len(files) == 0 {
		return nil, fmt.Errorf("%w: no goggame-*.info file found in %s or %s/game", ErrMetadataNotFound, c.workspace, c.workspace)
	}

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			c.log.Debug("couldn't read info file", "file", f, "error", err)
			continue
		}

		var meta GameMetadata
		err = json.Unmarshal(data, &meta)
		if err != nil {
			c.log.Debug("skipping invalid info file", "file", f, "error", err)
			continue
		}

		if !meta.IsDLC {
			return &meta, nil
		}
	}

	return nil, fmt.Errorf("%w: could not identify base game metadata among info files in %s", ErrMetadataNotFound, c.workspace)
}
