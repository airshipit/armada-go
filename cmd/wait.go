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
	"context"

	"github.com/spf13/cobra"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"opendev.org/airship/armada-go/pkg/config"
	"opendev.org/airship/armada-operator/pkg/waitutil"
)

// NewWaitCommand creates a command to wait for armada manifests
func NewWaitCommand(_ config.Factory) *cobra.Command {
	getConfig := func() *rest.Config {
		k8sConfig, err := rest.InClusterConfig()
		if err != nil {
			k8sConfig, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
				clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{}).ClientConfig()
			if err != nil {
				panic(err)
			}
		}
		return k8sConfig
	}

	p := &waitutil.WaitOptions{
		RestConfig: getConfig(),
	}

	runCmd := &cobra.Command{
		Use:   "wait",
		Short: "armada-go command to wait for armada manifests",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			p.Logger = zap.New(zap.WriteTo(cmd.OutOrStdout()), zap.ConsoleEncoder())
			return p.Wait(context.Background())
		},
	}

	flags := runCmd.Flags()
	flags.StringVar(&p.ResourceType, "resource-type", "", "resource type")
	flags.StringVar(&p.Namespace, "namespace", "", "namespace")
	flags.StringVar(&p.LabelSelector, "label-selector", "", "label selector")
	flags.DurationVar(&p.Timeout, "timeout", 0, "timeout")
	flags.StringVar(&p.MinReady, "min-ready", "", "min ready")

	return runCmd
}
