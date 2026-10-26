package claudedesktop

import (
	"debug/elf"
	"errors"
	"os"
)

func verifyDesktopELF(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	binary, err := elf.NewFile(file)
	if err != nil {
		return errors.New("select the real Claude Desktop ELF executable; shell wrappers and the Claude CLI are unsupported")
	}
	defer binary.Close()
	if binary.Class != elf.ELFCLASS64 || (binary.Type != elf.ET_EXEC && binary.Type != elf.ET_DYN) || (binary.Machine != elf.EM_X86_64 && binary.Machine != elf.EM_AARCH64) {
		return errors.New("Claude Desktop ELF architecture or executable type is unsupported")
	}
	return nil
}
