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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/bingoohuang/ngg/ss"
)

type VisitorTransport struct {
	UseEncryption  bool `json:"useEncryption,omitempty"`
	UseCompression bool `json:"useCompression,omitempty"`
}

type VisitorBaseConfig struct {
	Name      string           `json:"name"`
	Type      string           `json:"type"`
	Transport VisitorTransport `json:"transport,omitempty"`
	SecretKey string           `json:"secretKey,omitempty"`
	// if the server user is not set, it defaults to the current user
	ServerUser string `json:"serverUser,omitempty"`
	ServerName string `json:"serverName,omitempty"`
	BindAddr   string `json:"bindAddr,omitempty"`
	// BindPort is the port that visitor listens on.
	// It can be less than 0, it means don't bind to the port and only receive connections redirected from
	// other visitors. (This is not supported for SUDP now)
	BindPort int `json:"bindPort,omitempty"`

	Users map[string]string `json:"users,omitempty"`
}

func (c *VisitorBaseConfig) GetBaseConfig() *VisitorBaseConfig {
	return c
}

func (c *VisitorBaseConfig) Complete(g *ClientCommonConfig) {
	if c.BindAddr == "" {
		c.BindAddr = "127.0.0.1"
	}

	namePrefix := ""
	if g.User != "" {
		namePrefix = g.User + "."
	}

	if c.ServerUser != "" {
		c.ServerName = c.ServerUser + "." + c.ServerName
	} else {
		c.ServerName = namePrefix + c.ServerName
	}

	c.Name = namePrefix + ss.Or(c.Name, c.ServerName)
}

type VisitorConfigurer interface {
	Complete(*ClientCommonConfig)
	GetBaseConfig() *VisitorBaseConfig
}

type VisitorType string

const (
	VisitorTypeSTCP VisitorType = "stcp"
	VisitorTypeSUDP VisitorType = "sudp"
)

var visitorConfigTypeMap = map[VisitorType]reflect.Type{
	VisitorTypeSTCP: reflect.TypeOf(STCPVisitorConfig{}),
	VisitorTypeSUDP: reflect.TypeOf(SUDPVisitorConfig{}),
}

type TypedVisitorConfig struct {
	Type string `json:"type"`
	VisitorConfigurer
}

func (c *TypedVisitorConfig) UnmarshalJSON(b []byte) error {
	if len(b) == 4 && string(b) == "null" {
		return errors.New("type is required")
	}

	typeStruct := struct {
		Type string `json:"type"`
	}{}
	if err := json.Unmarshal(b, &typeStruct); err != nil {
		return err
	}

	if typeStruct.Type == "" {
		typeStruct.Type = string(VisitorTypeSTCP)
	}

	c.Type = typeStruct.Type
	configurer := NewVisitorConfigurerByType(VisitorType(typeStruct.Type))
	if configurer == nil {
		return fmt.Errorf("unknown visitor type: %s", typeStruct.Type)
	}
	decoder := json.NewDecoder(bytes.NewBuffer(b))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(configurer); err != nil {
		return fmt.Errorf("unmarshal VisitorConfig error: %v", err)
	}
	c.VisitorConfigurer = configurer
	return nil
}

func (c *TypedVisitorConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.VisitorConfigurer)
}

func NewVisitorConfigurerByType(t VisitorType) VisitorConfigurer {
	v, ok := visitorConfigTypeMap[t]
	if !ok {
		return nil
	}
	vc := reflect.New(v).Interface().(VisitorConfigurer)
	vc.GetBaseConfig().Type = string(t)
	return vc
}

var _ VisitorConfigurer = &STCPVisitorConfig{}

type STCPVisitorConfig struct {
	VisitorBaseConfig
}

var _ VisitorConfigurer = &SUDPVisitorConfig{}

type SUDPVisitorConfig struct {
	VisitorBaseConfig
}
