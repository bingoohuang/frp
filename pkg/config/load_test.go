// Copyright 2023 The frp Authors
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

package config

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/fatedier/frp/pkg/config/v1"
)

const yamlServerContent = `
bindAddr: 127.0.0.1
kcpBindPort: 7000
quicBindPort: 7001
tcpmuxHTTPConnectPort: 7005
custom404Page: /abc.html
transport:
  tcpKeepalive: 10
`

func TestLoadServerConfig(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"yaml", yamlServerContent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require := require.New(t)
			svrCfg := v1.ServerConfig{}
			err := LoadConfigure([]byte(test.content), &svrCfg)
			require.NoError(err)
			require.EqualValues("127.0.0.1", svrCfg.BindAddr)
			require.EqualValues(7000, svrCfg.KCPBindPort)
			require.EqualValues(7001, svrCfg.QUICBindPort)
			require.EqualValues(7005, svrCfg.TCPMuxHTTPConnectPort)
			require.EqualValues("/abc.html", svrCfg.Custom404Page)
			require.EqualValues(10, svrCfg.Transport.TCPKeepAlive)
		})
	}
}

// Test that loading in strict mode fails when the config is invalid.
func TestLoadServerConfigStrictMode(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"yaml", yamlServerContent},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s-strict-%t", test.name, true), func(t *testing.T) {
			require := require.New(t)
			// Break the content with an innocent typo
			brokenContent := strings.Replace(test.content, "bindAddr", "bindAdur", 1)
			svrCfg := v1.ServerConfig{}
			err := LoadConfigure([]byte(brokenContent), &svrCfg)
			require.ErrorContains(err, "bindAdur")
		})
	}
}
