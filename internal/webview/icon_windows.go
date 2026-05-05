package webview

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/png"
	"os"
)

// WriteWindowsIcon writes a valid .ico file using the embedded app icon.
func WriteWindowsIcon(path string) error {
	config, err := png.DecodeConfig(bytes.NewReader(iconPNG))
	if err != nil {
		return fmt.Errorf("decode embedded icon: %w", err)
	}

	width := byte(config.Width)
	if config.Width >= 256 {
		width = 0
	}
	height := byte(config.Height)
	if config.Height >= 256 {
		height = 0
	}

	const iconDirSize = 6
	const iconEntrySize = 16
	const imageOffset = iconDirSize + iconEntrySize

	data := bytes.NewBuffer(make([]byte, 0, imageOffset+len(iconPNG)))
	write := func(value any) error {
		return binary.Write(data, binary.LittleEndian, value)
	}

	if err := write(uint16(0)); err != nil {
		return err
	}
	if err := write(uint16(1)); err != nil {
		return err
	}
	if err := write(uint16(1)); err != nil {
		return err
	}

	if err := data.WriteByte(width); err != nil {
		return err
	}
	if err := data.WriteByte(height); err != nil {
		return err
	}
	if err := data.WriteByte(0); err != nil {
		return err
	}
	if err := data.WriteByte(0); err != nil {
		return err
	}
	if err := write(uint16(1)); err != nil {
		return err
	}
	if err := write(uint16(32)); err != nil {
		return err
	}
	if err := write(uint32(len(iconPNG))); err != nil {
		return err
	}
	if err := write(uint32(imageOffset)); err != nil {
		return err
	}
	if _, err := data.Write(iconPNG); err != nil {
		return err
	}

	if err := os.WriteFile(path, data.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write icon file: %w", err)
	}
	return nil
}