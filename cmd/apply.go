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
	"github.com/spf13/cobra"

	"opendev.org/airship/armada-go/pkg/apply"
	"opendev.org/airship/armada-go/pkg/config"
)

// NewApplyCommand creates a command to apply armada manifests
func NewApplyCommand(cfgFactory config.Factory) *cobra.Command {
	p := &apply.RunCommand{Factory: cfgFactory}

	runCmd := &cobra.Command{
		Use:   "apply",
		Short: "armada-go command to apply manifests",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p.Manifests = args[0]
			p.Out = cmd.OutOrStdout()
			return p.RunE()
		},
	}

	var metricsOutput string
	flags := runCmd.Flags()
	flags.StringVar(&p.TargetManifest, "target-manifest", "", "target manifest")
	flags.StringVar(&metricsOutput, "metrics-output", "", "metrics output")

	return runCmd
}
