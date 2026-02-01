package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/amangsingh/agora/pkg/compiler"
	"github.com/spf13/cobra"
)

var outputDir string

// generateCmd represents the generate command
var generateCmd = &cobra.Command{
	Use:   "generate [blueprint-file]",
	Short: "Compile a blueprint into a Go project",
	Long: `Reads the specified YAML blueprint (default: agora.yaml) and
generates a full Go project in the output directory.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		blueprintPath := "agora.yaml"
		if len(args) > 0 {
			blueprintPath = args[0]
		}

		// Resolve output directory
		// If not specified, default to ./build
		if outputDir == "" {
			outputDir = "./build"
		}

		absOut, _ := filepath.Abs(outputDir)
		fmt.Printf("Generating project from '%s' into '%s'...\n", blueprintPath, absOut)

		if err := compiler.Compile(blueprintPath, outputDir); err != nil {
			return fmt.Errorf("compilation failed: %w", err)
		}

		fmt.Println("Done! \nTo run your agent:")
		fmt.Printf("  cd %s\n", outputDir)
		fmt.Println("  go mod tidy")
		fmt.Println("  go run .")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(generateCmd)

	generateCmd.Flags().StringVarP(&outputDir, "output", "o", "build", "Output directory for the generated project")
}
