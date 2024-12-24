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

package visitor

import (
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"time"

	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/msg"
	"github.com/fatedier/frp/pkg/socks5"
	netpkg "github.com/fatedier/frp/pkg/util/net"
	"github.com/fatedier/frp/pkg/util/util"
	"github.com/fatedier/frp/pkg/util/xlog"
	libio "github.com/fatedier/golib/io"
)

type STCPVisitor struct {
	*BaseVisitor

	cfg *v1.STCPVisitorConfig
}

func (sv *STCPVisitor) Run() (err error) {
	if sv.cfg.BindPort > 0 {
		sv.l, err = net.Listen("tcp", net.JoinHostPort(sv.cfg.BindAddr, strconv.Itoa(sv.cfg.BindPort)))
		if err != nil {
			return
		}
		go sv.worker()
	}

	go sv.internalConnWorker()
	return
}

func (sv *STCPVisitor) Close() {
	sv.BaseVisitor.Close()
}

func (sv *STCPVisitor) worker() {
	xl := xlog.FromContextSafe(sv.ctx)
	for {
		conn, err := sv.l.Accept()
		if err != nil {
			xl.Warnf("stcp local listener closed")
			return
		}
		go sv.handleConn(conn)
	}
}

func (sv *STCPVisitor) internalConnWorker() {
	xl := xlog.FromContextSafe(sv.ctx)
	for {
		conn, err := sv.internalLn.Accept()
		if err != nil {
			xl.Warnf("stcp internal listener closed")
			return
		}
		go sv.handleConn(conn)
	}
}

func (sv *STCPVisitor) handleConn(userConn net.Conn) {
	xl := xlog.FromContextSafe(sv.ctx)
	xl.Debugf("get a new stcp user connection")

	defer userConn.Close()

	var target string
	if sv.cfg.Socks5 {
		logger := log.New(os.Stderr, "[socks5] ", log.LstdFlags)
		socks5Srv := &socks5.Server{
			Logger: logger,
		}

		req, err := socks5Srv.ParseRequest(userConn)
		if err != nil {
			xl.Warnf("parse socks5 request error: %v", err)
			return
		}

		if req.Command != socks5.ConnectCommand {
			xl.Warnf("unsupported socks5 command: %v", req.Command)
			return
		}

		target = req.DestinationAddr.Address()
	} else {
		var err error
		target, err = netpkg.ParseTargetHead(userConn)
		if err != nil {
			return
		}
	}

	visitorConn, err := sv.helper.ConnectServer()
	if err != nil {
		return
	}
	defer visitorConn.Close()

	now := time.Now().Unix()
	newVisitorConnMsg := &msg.NewVisitorConn{
		RunID:          sv.helper.RunID(),
		ProxyName:      sv.cfg.ServerName,
		SignKey:        util.GetAuthKey(sv.cfg.SecretKey, now),
		Timestamp:      now,
		UseEncryption:  sv.cfg.Transport.UseEncryption,
		UseCompression: sv.cfg.Transport.UseCompression,

		TargetAddr: target,
	}
	if err := msg.WriteMsg(visitorConn, newVisitorConnMsg); err != nil {
		xl.Warnf("send newVisitorConnMsg to server error: %v", err)
		return
	}

	var newVisitorConnRespMsg msg.NewVisitorConnResp
	if err := msg.ReadMsgIntoTimeout(visitorConn, &newVisitorConnRespMsg, 10*time.Second); err != nil {
		xl.Warnf("get newVisitorConnRespMsg error: %v", err)
		return
	}

	if newVisitorConnRespMsg.Error != "" {
		xl.Warnf("start new visitor connection error: %s", newVisitorConnRespMsg.Error)
		return
	}

	var remote io.ReadWriteCloser
	remote = visitorConn
	if sv.cfg.Transport.UseEncryption {
		remote, err = libio.WithEncryption(remote, []byte(sv.cfg.SecretKey))
		if err != nil {
			xl.Errorf("create encryption stream error: %v", err)
			return
		}
	}

	if sv.cfg.Transport.UseCompression {
		var recycleFn func()
		remote, recycleFn = libio.WithCompressionFromPool(remote)
		defer recycleFn()
	}

	if sv.cfg.Socks5 {
		if err := socks5.SendSuccessReply(userConn, visitorConn); err != nil {
			xl.Warnf("send success reply error: %v", err)
			return
		}
	}

	libio.Join(userConn, remote)
}
