package main

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "fontpub",
	Short: "Decentralized & Verifiable Font Distribution",
	Long: `Fontpub is a CLI tool for managing font installation, activation,
and updates on macOS. It provides a secure and verifiable way to
distribute fonts from GitHub repositories.`,
}

func init() {
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(activateCmd)
	rootCmd.AddCommand(deactivateCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(statusCmd)
}
