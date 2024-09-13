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

package v1

import (
	"cmp"

	"github.com/fatedier/frp/pkg/util/util"
)

type AuthScope string

const (
	AuthScopeHeartBeats   AuthScope = "HeartBeats"
	AuthScopeNewWorkConns AuthScope = "NewWorkConns"
)

type AuthMethod string

const (
	AuthMethodToken AuthMethod = "token"
)

// QUIC protocol options
type QUICOptions struct {
	KeepalivePeriod    int `json:"keepalivePeriod,omitempty"`
	MaxIdleTimeout     int `json:"maxIdleTimeout,omitempty"`
	MaxIncomingStreams int `json:"maxIncomingStreams,omitempty"`
}

func (c *QUICOptions) Complete() {
	c.KeepalivePeriod = cmp.Or(c.KeepalivePeriod, 10)
	c.MaxIdleTimeout = cmp.Or(c.MaxIdleTimeout, 30)
	c.MaxIncomingStreams = cmp.Or(c.MaxIncomingStreams, 100000)
}

type TLSConfig struct {
	// CertPath specifies the path of the cert file that client will load.
	CertFile string `json:"certFile,omitempty"`
	// KeyPath specifies the path of the secret key file that client will load.
	KeyFile string `json:"keyFile,omitempty"`
	// TrustedCaFile specifies the path of the trusted ca file that will load.
	TrustedCaFile string `json:"trustedCaFile,omitempty"`
	// ServerName specifies the custom server name of tls certificate. By
	// default, server name if same to ServerAddr.
	ServerName string `json:"serverName,omitempty"`
}

type LogConfig struct {
	// This is destination where frp should write the logs.
	// If "console" is used, logs will be printed to stdout, otherwise,
	// logs will be written to the specified file.
	// By default, this value is "console".
	To string `json:"to,omitempty"`
	// Level specifies the minimum log level. Valid values are "trace",
	// "debug", "info", "warn", and "error". By default, this value is "info".
	Level string `json:"level,omitempty"`
	// MaxDays specifies the maximum number of days to store log information
	// before deletion.
	MaxDays int64 `json:"maxDays"`
	// DisablePrintColor disables log colors when log.to is "console".
	DisablePrintColor bool `json:"disablePrintColor,omitempty"`
}

func (c *LogConfig) Complete() {
	c.To = cmp.Or(util.ExpandFile(c.To), "console")
	c.Level = cmp.Or(c.Level, "info")
	c.MaxDays = cmp.Or(c.MaxDays, 3)
}

type HTTPPluginOptions struct {
	Name      string   `json:"name"`
	Addr      string   `json:"addr"`
	Path      string   `json:"path"`
	Ops       []string `json:"ops"`
	TLSVerify bool     `json:"tlsVerify,omitempty"`
}

type HeaderOperations struct {
	Set map[string]string `json:"set,omitempty"`
}

type HTTPHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
