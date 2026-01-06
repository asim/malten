package command

import (
	"malten.ai/spatial"
)

func init() {
	Register(&Command{
		Name:        "cleanup",
		Description: "Clean up expired and duplicate arrivals (runs in background)",
		Usage:       "/cleanup",
		Handler:     handleCleanup,
	})
}

func handleCleanup(ctx *Context, args []string) (string, error) {
	go func() {
		db := spatial.Get()
		db.CleanupStore()
	}()

	return "🧹 Cleanup started in background. Check logs for results.", nil
}
