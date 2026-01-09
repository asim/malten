package command

import (
	"fmt"
	"log"
	"strings"
)

// Context provides user context to commands
type Context struct {
	Session      string
	Stream       string // Geohash stream
	Lat          float64
	Lon          float64
	Accuracy     float64 // GPS accuracy in meters (0 if not provided)
	ToLat        float64 // Destination lat (for directions)
	ToLon        float64 // Destination lon (for directions)
	Input        string
	PushMessages []string // Messages to push via websocket after command completes
}

// HasLocation returns true if user has shared location
func (c *Context) HasLocation() bool {
	return c.Lat != 0 || c.Lon != 0
}

// Command represents a pluggable command handler
type Command struct {
	Name        string
	Description string
	Usage       string
	Emoji       string // Emoji for display (e.g., "🚶")
	LoadingText string // Text while loading (e.g., "Getting directions to %s...")
	Handler     func(ctx *Context, args []string) (string, error)
	Match       func(input string) (bool, []string) // optional natural language matcher
}

// Registry holds all registered commands
var Registry = make(map[string]*Command)

// Callbacks - set by main.go to avoid import cycle
var (
	ResetSessionCallback func(stream, sessionID string) int // Returns cleared message count
)

// Register adds a command to the registry
func Register(cmd *Command) {
	Registry[cmd.Name] = cmd
}

// Get returns a command by name
func Get(name string) *Command {
	return Registry[name]
}

// List returns all command names
func List() []string {
	var names []string
	for name := range Registry {
		names = append(names, name)
	}
	return names
}

// CommandMeta is the client-facing command metadata
type CommandMeta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Usage       string `json:"usage,omitempty"`
	Emoji       string `json:"emoji,omitempty"`
	LoadingText string `json:"loading,omitempty"`
}

// GetMeta returns metadata for all commands (for client)
func GetMeta() []CommandMeta {
	var meta []CommandMeta
	for _, cmd := range Registry {
		meta = append(meta, CommandMeta{
			Name:        cmd.Name,
			Description: cmd.Description,
			Usage:       cmd.Usage,
			Emoji:       cmd.Emoji,
			LoadingText: cmd.LoadingText,
		})
	}
	return meta
}

// Dispatch routes input to the appropriate command
// Returns (result, handled) - handled=false means fall through to AI
func Dispatch(ctx *Context) (string, bool) {
	input := strings.TrimSpace(ctx.Input)

	// Slash commands: /name args
	if strings.HasPrefix(input, "/") {
		parts := strings.Fields(input)
		name := strings.TrimPrefix(parts[0], "/")
		args := parts[1:]
		if cmd := Get(name); cmd != nil {
			result, err := cmd.Handler(ctx, args)
			if err != nil {
				return "❌ " + err.Error(), true
			}
			return result, true
		}
		// Unknown slash command
		return "❌ Unknown command: /" + name, true
	}

	// Natural language: check each command's Match function
	for _, cmd := range Registry {
		if cmd.Match != nil {
			if matched, args := cmd.Match(input); matched {
				log.Printf("[command] %s matched input %q with args %v", cmd.Name, input, args)
				result, err := cmd.Handler(ctx, args)
				if err != nil {
					return "❌ " + err.Error(), true
				}
				return result, true
			}
		}
	}

	return "", false
}

// Execute runs a command by name with args (legacy API for agent tools)
// Deprecated: use Dispatch instead
func Execute(name string, args []string) (string, error) {
	ctx := &Context{Input: "/" + name + " " + strings.Join(args, " ")}
	result, handled := Dispatch(ctx)
	if !handled {
		return "", nil
	}
	if strings.HasPrefix(result, "\u274c ") {
		return "", fmt.Errorf(strings.TrimPrefix(result, "\u274c "))
	}
	return result, nil
}
