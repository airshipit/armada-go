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

	"opendev.org/airship/armada-go/pkg/config"
	"opendev.org/airship/armada-go/pkg/server"
)

const (
	runLong = `
Run armada-go in server mode
`
	runExample = `
Run armada-go server
# armada-go server
`
)

// NewServerCommand creates a command to run specific phase
func NewServerCommand(cfgFactory config.Factory) *cobra.Command {
	p := &server.RunCommand{Factory: cfgFactory}

	runCmd := &cobra.Command{
		Use:     "server",
		Short:   "armada-go command to run server",
		Long:    runLong[1:],
		Args:    cobra.ExactArgs(0),
		Example: runExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return p.RunE()
		},
	}

	return runCmd
}
