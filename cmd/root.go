/*
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     https://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package cmd

import (
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	cfg "opendev.org/airship/armada-go/pkg/config"
	"opendev.org/airship/armada-go/pkg/log"
)

const longRoot = `Armada-Go is a tool for managing multiple Helm charts with dependencies by centralizing 
all configurations in a single Armada YAML and providing life-cycle hooks for all Helm releases.`

// RootOptions stores global flags values
type RootOptions struct {
	Debug            bool
	ArmadaConfigPath string
}

// NewArmadaCommand creates a root `armada` command with the default commands attached
func NewArmadaCommand(out io.Writer) *cobra.Command {
	rootCmd, settings := NewRootCommand(out)
	return AddDefaultArmadaCommands(rootCmd,
		cfg.CreateFactory(&settings.ArmadaConfigPath))
}

// NewRootCommand creates the root `armada` command. All other commands are
// subcommands branching from this one
func NewRootCommand(out io.Writer) (*cobra.Command, *RootOptions) {
	options := &RootOptions{}
	rootCmd := &cobra.Command{
		Use:           "armada",
		Short:         "A Golang-based orchestrator for managing a collection of Kubernetes Helm charts",
		Long:          longRoot,
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			log.Init(options.Debug, cmd.ErrOrStderr())
		},
	}
	rootCmd.SetOut(out)
	initFlags(options, rootCmd)

	return rootCmd, options
}

// AddDefaultArmadaCommands is a convenience function for adding all the
// default commands to armada-go
func AddDefaultArmadaCommands(cmd *cobra.Command, factory cfg.Factory) *cobra.Command {
	cmd.AddCommand(NewServerCommand(factory))
	cmd.AddCommand(NewApplyCommand(factory))
	cmd.AddCommand(NewWaitCommand(factory))

	return cmd
}

func initFlags(options *RootOptions, cmd *cobra.Command) {
	flags := cmd.PersistentFlags()
	flags.BoolVar(&options.Debug, "debug", false, "enable verbose output")

	defaultArmadaConfigDir := filepath.Join("$HOME", ".armada")

	defaultArmadaConfigPath := filepath.Join(defaultArmadaConfigDir, "config")
	flags.StringVar(&options.ArmadaConfigPath, "armadaconf", "",
		`path to the armada-go configuration file. Defaults to "`+defaultArmadaConfigPath+`"`)
}
