package command

import (
	"fmt"
	"log"
	"strings"
)

// Context provides user context to commands
type Context struct {
	Session string
	Lat     float64
	Lon     float64
	Input   string
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
	Handler     func(ctx *Context, args []string) (string, error)
	Match       func(input string) (bool, []string) // optional natural language matcher
}

// Registry holds all registered commands
var Registry = make(map[string]*Command)

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
