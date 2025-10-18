// Package cli provides utilities for building command-line interfaces from wrapped functions.
//
// # Overview
//
// The cli package enables you to create command dispatchers that parse command-line
// arguments and execute wrapped functions. It supports both single-level and multi-level
// command hierarchies, automatic help text generation, and shell completion.
//
// # Basic Usage
//
// Create a simple CLI with commands:
//
//	func Deploy(env, service string, version int) error {
//	    fmt.Printf("Deploying %s v%d to %s\n", service, version, env)
//	    return nil
//	}
//
//	func main() {
//	    dispatcher := cli.NewStringArgsDispatcher("myapp")
//	    dispatcher.MustAddCommand("deploy", "Deploy a service",
//	        function.MustReflectWrapper("deploy", Deploy))
//
//	    err := dispatcher.DispatchCombinedCommandAndArgs(context.Background(), os.Args[1:])
//	    if err != nil {
//	        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
//	        os.Exit(1)
//	    }
//	}
//
// Usage:
//
//	$ myapp deploy production api-server 42
//	Deploying api-server v42 to production
//
// # Multi-level Commands
//
// Create nested command hierarchies:
//
//	dispatcher := cli.NewSuperStringArgsDispatcher("myapp")
//
//	// Add user commands
//	userCmd := dispatcher.MustAddSuperCommand("user")
//	userCmd.MustAddCommand("create", "Create a new user",
//	    function.MustReflectWrapper("CreateUser", CreateUser))
//	userCmd.MustAddCommand("delete", "Delete a user",
//	    function.MustReflectWrapper("DeleteUser", DeleteUser))
//
//	// Add db commands
//	dbCmd := dispatcher.MustAddSuperCommand("db")
//	dbCmd.MustAddCommand("migrate", "Run migrations",
//	    function.MustReflectWrapper("Migrate", Migrate))
//
//	dispatcher.DispatchCombinedCommandAndArgs(context.Background(), os.Args[1:])
//
// Usage:
//
//	$ myapp user create alice alice@example.com
//	$ myapp user delete alice
//	$ myapp db migrate
//
// # Default Commands
//
// You can register a default command that runs when no command is specified:
//
//	dispatcher.MustAddDefaultCommand("Run the server",
//	    function.MustReflectWrapper("RunServer", RunServer))
//
// Now running the program without arguments will execute the default command:
//
//	$ myapp
//	Server started on :8080
//
// # Help and Usage
//
// Automatically print available commands:
//
//	if len(os.Args) == 1 || os.Args[1] == "help" {
//	    dispatcher.PrintCommandsUsageIntro()
//	    os.Exit(0)
//	}
//
// This prints formatted help text with command names, argument types, and descriptions.
//
// # Shell Completion
//
// Enable shell completion for your CLI:
//
//	func main() {
//	    dispatcher := cli.NewStringArgsDispatcher("myapp")
//	    // ... add commands ...
//
//	    // Enable completion
//	    cli.CompleteStringArgsDispatcher(dispatcher)
//
//	    // Normal dispatch logic
//	    dispatcher.DispatchCombinedCommandAndArgs(context.Background(), os.Args[1:])
//	}
//
// Users can then install completion:
//
//	$ myapp --install-completion
//	$ myapp de<TAB>  # completes to "deploy"
//
// # Logging
//
// Track command execution with loggers:
//
//	logger := cli.StringArgsCommandLoggerFunc(func(command string, args []string) {
//	    log.Printf("Executing: %s with args %v", command, args)
//	})
//
//	dispatcher := cli.NewStringArgsDispatcher("myapp", logger)
//
// # Customization
//
// Customize output colors:
//
//	import "github.com/fatih/color"
//
//	cli.UsageColor = color.New(color.FgGreen)
//	cli.DescriptionColor = color.New(color.FgYellow)
//
// # Error Handling
//
// The package provides specific error types for better error handling:
//
//	err := dispatcher.Dispatch(ctx, "unknown-command")
//	if cli.IsErrCommandNotFound(err) {
//	    fmt.Println("Command not found. Available commands:")
//	    dispatcher.PrintCommands()
//	}
//
// # Result Handlers
//
// Process function results before they're displayed:
//
//	resultHandler := function.ResultsHandlerFunc(func(results []any) ([]any, error) {
//	    // Transform or validate results
//	    return results, nil
//	})
//
//	dispatcher.MustAddCommand("query", "Query the database",
//	    wrapper, resultHandler)
//
// # Best Practices
//
//   - Use descriptive command names (verbs like "create", "delete", "list")
//   - Provide clear descriptions for commands and arguments
//   - Handle errors gracefully with appropriate exit codes
//   - Use default commands sparingly (only for single-purpose CLIs)
//   - Enable shell completion for better user experience
//   - Log command execution in production applications
package cli
