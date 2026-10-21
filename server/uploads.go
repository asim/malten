package server

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"strings"
)

const MaxPhotoBytes = 512 << 10

func validatePhoto(value string) error {
	if value == "" {
		return nil
	}
	header, data, ok := strings.Cut(value, ",")
	if !ok || (header != "data:image/jpeg;base64" && header != "data:image/png;base64") || len(data) > base64.StdEncoding.EncodedLen(MaxPhotoBytes) {
		return errors.New("Photo must be a JPEG or PNG of 512 KB or less")
	}
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil || len(decoded) > MaxPhotoBytes {
		return errors.New("Invalid photo")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(decoded))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 4096 || cfg.Height > 4096 || header != "data:image/"+format+";base64" {
		return errors.New("Invalid photo dimensions or format")
	}
	return nil
}
