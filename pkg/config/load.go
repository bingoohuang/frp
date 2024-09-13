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
	"bytes"
	"cmp"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/yaml"

	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/config/v1/validation"
	"github.com/fatedier/frp/pkg/msg"
	"github.com/fatedier/frp/pkg/util/util"
)

var glbEnvs map[string]string

func init() {
	glbEnvs = make(map[string]string)
	envs := os.Environ()
	for _, env := range envs {
		pair := strings.SplitN(env, "=", 2)
		if len(pair) != 2 {
			continue
		}
		glbEnvs[pair[0]] = pair[1]
	}
}

type Values struct {
	Envs map[string]string // environment vars
}

func GetValues() *Values {
	return &Values{
		Envs: glbEnvs,
	}
}

func RenderWithTemplate(in []byte, values *Values) ([]byte, error) {
	tmpl, err := template.New("frp").Funcs(template.FuncMap{
		"parseNumberRange":     parseNumberRange,
		"parseNumberRangePair": parseNumberRangePair,
	}).Parse(string(in))
	if err != nil {
		return nil, err
	}

	buffer := bytes.NewBufferString("")
	if err := tmpl.Execute(buffer, values); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func LoadFileContentWithTemplate(path string, values *Values) ([]byte, error) {
	b, err := os.ReadFile(util.ExpandFile(path))
	if err != nil {
		return nil, err
	}
	return RenderWithTemplate(b, values)
}

func LoadConfigureFromFile(path string, c any) error {
	content, err := LoadFileContentWithTemplate(path, GetValues())
	if err != nil {
		return err
	}
	return LoadConfigure(content, c)
}

// LoadConfigure loads configuration from bytes and unmarshal into c.
// Now it only supports yaml format.
func LoadConfigure(b []byte, c any) error {
	return yaml.UnmarshalStrict(b, c)
}

func NewProxyConfigurerFromMsg(m *msg.NewProxy, serverCfg *v1.ServerConfig) (v1.ProxyConfigurer, error) {
	m.ProxyType = cmp.Or(m.ProxyType, string(v1.ProxyTypeTCP))

	configurer := v1.NewProxyConfigurerByType(v1.ProxyType(m.ProxyType))
	if configurer == nil {
		return nil, fmt.Errorf("unknown proxy type: %s", m.ProxyType)
	}

	configurer.UnmarshalFromMsg(m)
	configurer.Complete("")

	if err := validation.ValidateProxyConfigurerForServer(configurer, serverCfg); err != nil {
		return nil, err
	}
	return configurer, nil
}

func LoadServerConfig(path string) (*v1.ServerConfig, error) {
	svrCfg := &v1.ServerConfig{}
	if err := LoadConfigureFromFile(path, svrCfg); err != nil {
		return nil, err
	}

	svrCfg.Complete()
	return svrCfg, nil
}

func LoadClientConfig(path string) (
	*v1.ClientCommonConfig,
	[]v1.ProxyConfigurer,
	[]v1.VisitorConfigurer,
	error,
) {
	var (
		cliCfg      *v1.ClientCommonConfig
		proxyCfgs   = make([]v1.ProxyConfigurer, 0)
		visitorCfgs = make([]v1.VisitorConfigurer, 0)
	)

	allCfg := v1.ClientConfig{}
	if err := LoadConfigureFromFile(path, &allCfg); err != nil {
		return nil, nil, nil, err
	}
	cliCfg = &allCfg.ClientCommonConfig
	for _, c := range allCfg.Proxies {
		proxyCfgs = append(proxyCfgs, c.ProxyConfigurer)
	}
	for _, c := range allCfg.Visitors {
		visitorCfgs = append(visitorCfgs, c.VisitorConfigurer)
	}

	// Load additional config from includes.
	// legacy ini format already handle this in ParseClientConfig.
	if len(cliCfg.IncludeConfigFiles) > 0 {
		extProxyCfgs, extVisitorCfgs, err := LoadAdditionalClientConfigs(cliCfg.IncludeConfigFiles)
		if err != nil {
			return nil, nil, nil, err
		}
		proxyCfgs = append(proxyCfgs, extProxyCfgs...)
		visitorCfgs = append(visitorCfgs, extVisitorCfgs...)
	}

	// Filter by start
	if len(cliCfg.Start) > 0 {
		startSet := sets.New(cliCfg.Start...)
		proxyCfgs = lo.Filter(proxyCfgs, func(c v1.ProxyConfigurer, _ int) bool {
			return startSet.Has(c.GetBaseConfig().Name)
		})
		visitorCfgs = lo.Filter(visitorCfgs, func(c v1.VisitorConfigurer, _ int) bool {
			return startSet.Has(c.GetBaseConfig().Name)
		})
	}

	cliCfg.Complete()

	for _, c := range proxyCfgs {
		c.Complete(cliCfg.User)
	}
	for _, c := range visitorCfgs {
		c.Complete(cliCfg)
	}
	return cliCfg, proxyCfgs, visitorCfgs, nil
}

func LoadAdditionalClientConfigs(paths []string) ([]v1.ProxyConfigurer, []v1.VisitorConfigurer, error) {
	proxyCfgs := make([]v1.ProxyConfigurer, 0)
	visitorCfgs := make([]v1.VisitorConfigurer, 0)
	for _, path := range paths {
		absDir, err := filepath.Abs(filepath.Dir(path))
		if err != nil {
			return nil, nil, err
		}
		if _, err := os.Stat(absDir); os.IsNotExist(err) {
			return nil, nil, err
		}
		files, err := os.ReadDir(absDir)
		if err != nil {
			return nil, nil, err
		}
		for _, fi := range files {
			if fi.IsDir() {
				continue
			}
			absFile := filepath.Join(absDir, fi.Name())
			if matched, _ := filepath.Match(filepath.Join(absDir, filepath.Base(path)), absFile); matched {
				// support yaml/json/toml
				cfg := v1.ClientConfig{}
				if err := LoadConfigureFromFile(absFile, &cfg); err != nil {
					return nil, nil, fmt.Errorf("load additional config from %s error: %v", absFile, err)
				}
				for _, c := range cfg.Proxies {
					proxyCfgs = append(proxyCfgs, c.ProxyConfigurer)
				}
				for _, c := range cfg.Visitors {
					visitorCfgs = append(visitorCfgs, c.VisitorConfigurer)
				}
			}
		}
	}
	return proxyCfgs, visitorCfgs, nil
}
