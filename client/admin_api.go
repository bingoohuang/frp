// Copyright 2017 fatedier, fatedier@gmail.com
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

package client

import (
	"cmp"
	"encoding/json"
	"fmt"
	"github.com/fatedier/frp/client/proxy"
	"github.com/fatedier/frp/pkg/util/log"
	"io"
	"net"
	"net/http"
	"os"
	"slices"
	"strconv"
)

type GeneralResponse struct {
	Code int
	Msg  string
}

type StatusResp map[string][]ProxyStatusResp

type ProxyStatusResp struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	Err        string `json:"err"`
	LocalAddr  string `json:"local_addr"`
	Plugin     string `json:"plugin"`
	RemoteAddr string `json:"remote_addr"`
}

func NewProxyStatusResp(status *proxy.WorkingStatus, serverAddr string) ProxyStatusResp {
	psr := ProxyStatusResp{
		Name:   status.Name,
		Type:   status.Type,
		Status: status.Phase,
		Err:    status.Err,
	}
	baseCfg := status.Cfg.GetBaseConfig()
	if baseCfg.LocalPort != 0 {
		psr.LocalAddr = net.JoinHostPort(baseCfg.LocalIP, strconv.Itoa(baseCfg.LocalPort))
	}
	psr.Plugin = baseCfg.Plugin.Type

	if status.Err == "" {
		psr.RemoteAddr = status.RemoteAddr
		if slices.Contains([]string{"tcp", "udp"}, status.Type) {
			psr.RemoteAddr = serverAddr + psr.RemoteAddr
		}
	}
	return psr
}

// GET /api/status
func (svr *Service) apiStatus(w http.ResponseWriter, _ *http.Request) {
	var (
		buf []byte
		res StatusResp = make(map[string][]ProxyStatusResp)
	)

	log.Infof("Http request [/api/status]")
	defer func() {
		log.Infof("Http response [/api/status]")
		buf, _ = json.Marshal(&res)
		_, _ = w.Write(buf)
	}()

	svr.ctlMu.RLock()
	ctl := svr.ctl
	svr.ctlMu.RUnlock()
	if ctl == nil {
		return
	}

	ps := ctl.pm.GetAllProxyStatus()
	for _, status := range ps {
		res[status.Type] = append(res[status.Type], NewProxyStatusResp(status, svr.common.ServerAddr))
	}

	for _, arrs := range res {
		if len(arrs) <= 1 {
			continue
		}
		slices.SortFunc(arrs, func(a, b ProxyStatusResp) int {
			return cmp.Compare(a.Name, b.Name)
		})
	}
}

// GET /api/config
func (svr *Service) apiGetConfig(w http.ResponseWriter, _ *http.Request) {
	res := GeneralResponse{Code: 200}

	log.Infof("Http get request [/api/config]")
	defer func() {
		log.Infof("Http get response [/api/config], code [%d]", res.Code)
		w.WriteHeader(res.Code)
		if len(res.Msg) > 0 {
			_, _ = w.Write([]byte(res.Msg))
		}
	}()

	if svr.configFilePath == "" {
		res.Code = 400
		res.Msg = "frpc has no config file path"
		log.Warnf("%s", res.Msg)
		return
	}

	content, err := os.ReadFile(svr.configFilePath)
	if err != nil {
		res.Code = 400
		res.Msg = err.Error()
		log.Warnf("load frpc config file error: %s", res.Msg)
		return
	}
	res.Msg = string(content)
}

// PUT /api/config
func (svr *Service) apiPutConfig(w http.ResponseWriter, r *http.Request) {
	res := GeneralResponse{Code: 200}

	log.Infof("Http put request [/api/config]")
	defer func() {
		log.Infof("Http put response [/api/config], code [%d]", res.Code)
		w.WriteHeader(res.Code)
		if len(res.Msg) > 0 {
			_, _ = w.Write([]byte(res.Msg))
		}
	}()

	// get new config content
	body, err := io.ReadAll(r.Body)
	if err != nil {
		res.Code = 400
		res.Msg = fmt.Sprintf("read request body error: %v", err)
		log.Warnf("%s", res.Msg)
		return
	}

	if len(body) == 0 {
		res.Code = 400
		res.Msg = "body can't be empty"
		log.Warnf("%s", res.Msg)
		return
	}

	if err := os.WriteFile(svr.configFilePath, body, 0o600); err != nil {
		res.Code = 500
		res.Msg = fmt.Sprintf("write content to frpc config file error: %v", err)
		log.Warnf("%s", res.Msg)
		return
	}
}
