package command

// Command represents a pluggable command handler
type Command struct {
	Name        string
	Description string
	Usage       string
	Handler     func(args []string) (string, error)
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

// Execute runs a command by name with args
func Execute(name string, args []string) (string, error) {
	cmd := Get(name)
	if cmd == nil {
		return "", nil // not found, let it fall through
	}
	return cmd.Handler(args)
}
