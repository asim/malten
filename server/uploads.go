package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"strings"
)

const MaxPhotoBytes = 512 << 10

// Moderate is configured once at startup. Device-local entries never reach it.
var Moderate func(context.Context, *Message) error
var ErrModerationRejected = errors.New("This post cannot be shared in a public stream. You can edit it and try again.")
var ErrModerationUnavailable = errors.New("Moderation is unavailable. Your post has not been sent. Try again shortly.")

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

func moderatePost(w http.ResponseWriter, r *http.Request, message *Message) bool {
	if Moderate == nil {
		return true
	}
	if err := Moderate(r.Context(), message); err != nil {
		code := http.StatusServiceUnavailable
		if errors.Is(err, ErrModerationRejected) {
			code = http.StatusUnprocessableEntity
		}
		if code == http.StatusServiceUnavailable {
			err = ErrModerationUnavailable
		}
		http.Error(w, err.Error(), code)
		return false
	}
	return true
}
