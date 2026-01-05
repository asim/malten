package command

import (
	"fmt"
	"runtime"
	"time"

	"malten.ai/spatial"
)

func init() {
	Register(&Command{
		Name:        "status",
		Description: "Server status and debug info",
		Handler:     handleStatus,
	})
}

func handleStatus(ctx *Context, args []string) (string, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	db := spatial.Get()
	stats := db.Stats()

	// Format memory in MB
	allocMB := float64(m.Alloc) / 1024 / 1024
	sysMB := float64(m.Sys) / 1024 / 1024
	
	return fmt.Sprintf(`🔧 Server Status

**Memory**
• Alloc: %.1f MB
• Sys: %.1f MB
• GC cycles: %d

**Spatial DB**
• Entities: %d
• Agents: %d
• Weather: %d
• Prayer: %d
• Arrivals: %d
• Places: %d

**Uptime**: %s`,
		allocMB, sysMB, m.NumGC,
		stats.Total, stats.Agents, stats.Weather, stats.Prayer, stats.Arrivals, stats.Places,
		time.Since(startTime).Round(time.Second),
	), nil
}

var startTime = time.Now()
