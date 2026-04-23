package cli

import "fmt"

// Execute dispatches to the appropriate command handler based on args.
// Defaults to "generate" if no command is given.
func Execute(args []string) error {
	cmd := "generate"
	if len(args) > 0 && args[0] != "" {
		cmd = args[0]
		args = args[1:]
	}

	switch cmd {
	case "generate":
		return runGenerate(args)
	case "validate":
		return runValidate(args)
	default:
		return fmt.Errorf("unknown command %q; valid commands: generate, validate", cmd)
	}
}
