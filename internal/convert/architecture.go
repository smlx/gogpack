package convert

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// checkArchitecture reads the ELF header of the main executable to check if it's 32-bit (x86).
func (c *Converter) checkArchitecture() (bool, error) {
	fullPath := filepath.Join(c.workspace, c.mainExe)

	f, err := os.Open(fullPath)
	if err != nil {
		return false, fmt.Errorf("could not open main executable %s: %w", fullPath, err)
	}
	defer f.Close()

	header := make([]byte, 20) // ELF header machine type is at offset 18
	n, err := f.Read(header)
	if err != nil {
		return false, fmt.Errorf("could not read ELF header of %s: %w", fullPath, err)
	}

	elfMagic := []byte{0x7F, 'E', 'L', 'F'}
	if n < 20 || !bytes.Equal(header[:4], elfMagic) {
		// Not an ELF binary (maybe a shell script wrapper)
		return false, nil
	}

	// Machine field is at offset 18 (2 bytes). 0x03 is EM_386 (Intel 80386)
	is32Bit := header[18] == 0x03 && header[19] == 0x00

	return is32Bit, nil
}
