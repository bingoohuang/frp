// Copyright 2018 fatedier, fatedier@gmail.com
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"os"

	_ "github.com/bingoohuang/ngg/daemon/autoload"
	"github.com/bingoohuang/ngg/ver"
	"github.com/fatedier/frp"
	"github.com/fatedier/frp/pkg/util/system"
	"github.com/spf13/cobra"
)

func main() {
	system.EnableCompatibilityMode()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var (
	cfgFile     string
	showVersion bool
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file of frp")
	rootCmd.PersistentFlags().BoolVarP(&showVersion, "version", "v", false, "version of frp")
}

var rootCmd = &cobra.Command{
	Use: "frp",
	RunE: func(cmd *cobra.Command, args []string) error {
		if showVersion {
			fmt.Println(ver.Version())
			return nil
		}

		return frp.Run(cfgFile)
	},
}
