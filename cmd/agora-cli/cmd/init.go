package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init [project-name]",
	Short: "Initialize a new Agora project",
	Long: `Scaffolds a new Agora project with a default blueprint (agora.yaml)
and a go.mod file.

Example:
  agora-cli init my-agent`,
	RunE: func(cmd *cobra.Command, args []string) error {
		projectName := "my-agent"
		if len(args) > 0 {
			projectName = args[0]
		}

		// 1. Check if files exist to prevent overwrite
		if _, err := os.Stat("agora.yaml"); err == nil {
			return fmt.Errorf("agora.yaml already exists in this directory")
		}
		if _, err := os.Stat("go.mod"); err == nil {
			return fmt.Errorf("go.mod already exists in this directory")
		}

		fmt.Printf("Initializing Agora project '%s'...\n", projectName)

		// 2. Create agora.yaml blueprint
		blueprintContent := fmt.Sprintf(`# Agora Blueprint v1.0
project: %s
version: 0.1.0

# Graph Configuration
graph:
  entry: agent
  max_steps: 25

# Nodes Definition
nodes:
  - name: agent
    type: agent
    model: llama3
    instructions: "You are a helpful AI assistant built with Agora."

# Edges (Simple Transition)
edges:
  - from: agent
    to: END
`, projectName)

		if err := os.WriteFile("agora.yaml", []byte(blueprintContent), 0644); err != nil {
			return fmt.Errorf("failed to write agora.yaml: %w", err)
		}
		fmt.Println("  [+] Created agora.yaml")

		// 3. Create go.mod
		goModContent := fmt.Sprintf(`module %s

go 1.25

require (
	github.com/amangsingh/agora v0.0.0
)
`, projectName)

		if err := os.WriteFile("go.mod", []byte(goModContent), 0644); err != nil {
			return fmt.Errorf("failed to write go.mod: %w", err)
		}
		fmt.Println("  [+] Created go.mod")

		fmt.Println("\nSuccess! To build your agent:")
		fmt.Println("  agora-cli generate")
		fmt.Println("  go mod tidy")
		fmt.Println("  go run .")

		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
