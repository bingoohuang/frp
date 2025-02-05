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
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/fatedier/frp/pkg/cmux"
	"github.com/fatedier/frp/pkg/cmux/pattern"
	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/httpproxy"
	"github.com/fatedier/frp/pkg/msg"
	"github.com/fatedier/frp/pkg/socks4"
	"github.com/fatedier/frp/pkg/socks5"
	"github.com/fatedier/frp/pkg/trie"
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

// HandlePrefix handle the handler that matches the prefix
func HandlePrefix(handlerTrie *trie.Trie[string], handler string, prefixes ...string) {
	for _, prefix := range prefixes {
		handlerTrie.Put([]byte(prefix), handler)
	}
}

var muxer = func() func(conn net.Conn) (string, net.Conn, error) {
	handlerTrie := trie.NewTrie[string]()

	HandlePrefix(handlerTrie, "http", append(pattern.Pattern[pattern.HTTP], pattern.Pattern[pattern.HTTP2]...)...)
	HandlePrefix(handlerTrie, "socks5", pattern.Pattern[pattern.SOCKS5]...)
	HandlePrefix(handlerTrie, "socks4", pattern.Pattern[pattern.SOCKS4]...)
	HandlePrefix(handlerTrie, "target", "TARGET ")

	return func(conn net.Conn) (string, net.Conn, error) {
		handler, buf, err := handlerTrie.MatchWithReader(conn)
		if err != nil {
			return "", nil, err
		}
		c := cmux.UnreadConn(conn, buf)
		return handler, c, nil
	}
}()

func (sv *STCPVisitor) newSocks4ServeConn() (ServeConn, error) {
	s, err := socks4.NewSimpleServer("socks4://:12345")
	if err != nil {
		return nil, err
	}
	if len(sv.cfg.Users) > 0 {
		s.Authentication = socks4.AuthenticationFunc(func(cmd socks4.Command, username string) bool {
			_, ok := sv.cfg.Users[username]
			return ok
		})
	}
	s.Context = sv.ctx
	s.ProxyDial = sv.DialContext

	return s, nil
}

func (sv *STCPVisitor) newSocks5ServeConn() (ServeConn, error) {
	s, err := socks5.NewSimpleServer("socks5://:12345")
	if err != nil {
		return nil, err
	}
	if len(sv.cfg.Users) > 0 {
		s.Authentication = socks5.AuthenticationFunc(func(cmd socks5.Command, username, password string) bool {
			return sv.cfg.Users[username] == password
		})
	}
	s.Context = sv.ctx
	s.ProxyDial = sv.DialContext
	return s, nil
}

func (sv *STCPVisitor) newHttpServeConn() (ServeConn, error) {
	s, err := httpproxy.NewSimpleServer("http://:12345")
	if err != nil {
		return nil, err
	}
	s.Server.BaseContext = func(listener net.Listener) context.Context {
		return sv.ctx
	}
	if len(sv.cfg.Users) > 0 {
		s.Authentication = httpproxy.BasicAuthFunc(func(username, password string) bool {
			return sv.cfg.Users[username] == password
		})
	}

	s.ProxyDial = sv.DialContext
	return NewHttpServeConn(&s.Server), nil
}

func (sv *STCPVisitor) DialContext(ctx context.Context, network, target string) (net.Conn, error) {
	xl := xlog.FromContextSafe(sv.ctx)
	visitorConn, err := sv.helper.ConnectServer()
	if err != nil {
		return nil, err
	}

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
		return nil, err
	}

	var newVisitorConnRespMsg msg.NewVisitorConnResp
	if err := msg.ReadMsgIntoTimeout(visitorConn, &newVisitorConnRespMsg, 10*time.Second); err != nil {
		xl.Warnf("get newVisitorConnRespMsg error: %v", err)
		return nil, err
	}

	if newVisitorConnRespMsg.Error != "" {
		xl.Warnf("start new visitor connection error: %s", newVisitorConnRespMsg.Error)
		return nil, fmt.Errorf("error: %s", newVisitorConnRespMsg.Error)
	}

	var remote io.ReadWriteCloser
	remote = visitorConn
	if sv.cfg.Transport.UseEncryption {
		remote, err = libio.WithEncryption(remote, []byte(sv.cfg.SecretKey))
		if err != nil {
			xl.Errorf("create encryption stream error: %v", err)
			return nil, err
		}
	}
	var recycleFn func()

	if sv.cfg.Transport.UseCompression {
		remote, recycleFn = libio.WithCompressionFromPool(remote)
	}

	return &readWriteCloserConn{ReadWriteCloser: remote, Original: visitorConn, recycleFn: recycleFn}, nil
}

type readWriteCloserConn struct {
	io.ReadWriteCloser
	Original  net.Conn
	recycleFn func()
}

// Add these methods to implement net.Conn interface
func (rwc *readWriteCloserConn) LocalAddr() net.Addr           { return rwc.Original.LocalAddr() }
func (rwc *readWriteCloserConn) RemoteAddr() net.Addr          { return rwc.Original.RemoteAddr() }
func (rwc *readWriteCloserConn) SetDeadline(t time.Time) error { return rwc.Original.SetDeadline(t) }
func (rwc *readWriteCloserConn) SetReadDeadline(t time.Time) error {
	return rwc.Original.SetReadDeadline(t)
}
func (rwc *readWriteCloserConn) SetWriteDeadline(t time.Time) error {
	return rwc.Original.SetWriteDeadline(t)
}

func (rwc *readWriteCloserConn) Close() error {
	if rwc.recycleFn != nil {
		rwc.recycleFn()
	}
	return rwc.Original.Close()
}

func (sv *STCPVisitor) handleConn(userConn net.Conn) {
	xl := xlog.FromContextSafe(sv.ctx)
	xl.Debugf("get a new stcp user connection")

	defer userConn.Close()

	proxyType, userConn, err := muxer(userConn)
	if err != nil {
		xl.Warnf("muxer error: %v", err)
		return
	}

	var target string
	switch proxyType {
	case "http":
		sc, err := sv.newHttpServeConn()
		if err != nil {
			xl.Warnf("new http serve conn error: %v", err)
			return
		}
		sc.ServeConn(userConn)
		return
	case "socks4":
		sc, err := sv.newSocks4ServeConn()
		if err != nil {
			xl.Warnf("new socks4 serve conn error: %v", err)
			return
		}
		sc.ServeConn(userConn)
		return
	case "socks5":
		sc, err := sv.newSocks5ServeConn()
		if err != nil {
			xl.Warnf("new socks5 serve conn error: %v", err)
			return
		}
		sc.ServeConn(userConn)
		return

	case "target":
		var err error
		target, err = netpkg.ParseTargetHead(userConn)
		if err != nil {
			return
		}
		remote, err := sv.DialContext(sv.ctx, "", target)
		if err != nil {
			xl.Warnf("dial context error: %v", err)
			return
		}
		defer remote.Close()

		libio.Join(userConn, remote)
	}
}
