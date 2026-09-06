package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print DNS-Proxy version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("DNS-Proxy v%s~%s\n", version, commit)
			fmt.Println("By Dimas Restu H <drh.dimasrestu@gmail.com>")
		},
	}
}
