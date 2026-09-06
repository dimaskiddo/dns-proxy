package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
)

var cfgFile string

func main() {
	os.Args = normalizeLegacyFlags(os.Args)

	rootCmd := &cobra.Command{
		Use:     "dns-proxy",
		Short:   "DNS Proxy: a simple DNS proxy/forwarder with DoT and DoH upstream support",
		Version: version + "~" + commit,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("DNS-Proxy v" + version + "~" + commit)
			fmt.Println("By Dimas Restu H <drh.dimasrestu@gmail.com>")
			fmt.Println("-------------------------------------------")

			return runServer(cfgFile)
		},
	}
	rootCmd.SetVersionTemplate("DNS-Proxy v{{.Version}}\nBy Dimas Restu H <drh.dimasrestu@gmail.com>\n")

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "./dns-proxy.yaml", "path to YAML configuration file")

	rootCmd.AddCommand(runCmd())
	rootCmd.AddCommand(versionCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Production predates the cobra migration and still invokes the binary with
// stdlib `flag` syntax, which pflag reads as a shorthand cluster instead
// (e.g. "-config" as -c -o -n -f -i -g) and fails to parse.
// covers only the two flags that ever shipped in the old binary — extend
// the set here if a new persistent long flag is ever added.
func normalizeLegacyFlags(args []string) []string {
	legacy := map[string]bool{"config": true, "version": true}

	out := make([]string, len(args))
	copy(out, args)

	for i, a := range out {
		if i == 0 || !strings.HasPrefix(a, "-") || strings.HasPrefix(a, "--") {
			continue
		}

		name, rest, hasEq := strings.Cut(a[1:], "=")
		if !legacy[name] {
			continue
		}

		if hasEq {
			out[i] = "--" + name + "=" + rest
		} else {
			out[i] = "--" + name
		}
	}

	return out
}
