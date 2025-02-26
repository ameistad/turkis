package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate completion script",
	Args:  cobra.ExactArgs(1),
	Long: `To load completions:

Bash:
  $ source <(turkis completion bash)
  # Permanently:
  $ turkis completion bash > /etc/bash_completion.d/turkis  # Linux
  $ turkis completion bash > /usr/local/etc/bash_completion.d/turkis  # macOS

Zsh:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc
  $ source <(turkis completion zsh)

Fish:
  $ turkis completion fish | source

Powershell:
  PS> turkis completion powershell | Out-String | Invoke-Expression
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return cmd.Root().GenBashCompletion(os.Stdout)
		case "zsh":
			return cmd.Root().GenZshCompletion(os.Stdout)
		case "fish":
			return cmd.Root().GenFishCompletion(os.Stdout, true)
		case "powershell":
			return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
		default:
			return fmt.Errorf("unsupported shell type: %s", args[0])
		}
	},
}

func init() {
	RootCmd.AddCommand(completionCmd)
}
