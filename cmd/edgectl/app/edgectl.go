/*
Copyright 2021 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package app

import (
	"github.com/spf13/cobra"

	"github.com/openyurtio/openyurt-operator/cmd/edgectl/app/cmd"
)

// NewEdgectlCommand returns cobra.Command to run edgectl command
func NewEdgectlCommand() *cobra.Command {
	cmds := &cobra.Command{
		Use:           "edgectl",
		Short:         "edgectl: easily convert or revert an node to edge or normal",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	cmds.AddCommand(cmd.NewCmdConvert())
	cmds.AddCommand(cmd.NewCmdRevert())

	return cmds
}
