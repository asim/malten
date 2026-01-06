package command

import (
	"fmt"
	"runtime"

	"malten.ai/spatial"
)

func init() {
	Register(&Command{
		Name:        "system",
		Description: "Show system status, API stats, and health",
		Usage:       "/system",
		Handler:     handleSystem,
	})
}

func handleSystem(ctx *Context, args []string) (string, error) {
	// Memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Entity counts
	db := spatial.Get()
	agents := db.ListAgents()

	var places, arrivals int
	// Count by type (approximation from agents' areas)
	for _, agent := range agents {
		p := db.Query(agent.Lat, agent.Lon, 2000, spatial.EntityPlace, 100)
		places += len(p)
		a := db.Query(agent.Lat, agent.Lon, 500, spatial.EntityArrival, 50)
		arrivals += len(a)
	}

	result := fmt.Sprintf(`🖥️ **System Status**

**Memory**
  Alloc: %.1f MB
  Sys: %.1f MB
  GC cycles: %d

**Agents**
  Total: %d
  Agentic mode: %v

**Entities** (approx)
  Places: %d
  Arrivals: %d

`,
		float64(m.Alloc)/1024/1024,
		float64(m.Sys)/1024/1024,
		m.NumGC,
		len(agents),
		spatial.AgenticMode,
		places,
		arrivals,
	)

	// API stats
	result += spatial.GetStats().Summary()

	return result, nil
}
