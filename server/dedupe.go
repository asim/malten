package server

import (
	"malten.ai/data"
)

// GetDedupe returns the notifications file (compatibility shim)
func GetDedupe() *data.NotificationsFile {
	return data.Notifications()
}

// ExtractContentKey delegates to data package
func ExtractContentKey(message string) string {
	return data.ExtractContentKey(message)
}
