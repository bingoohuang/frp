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

package msg

import (
	"io"
	"net"
	"time"

	jsonMsg "github.com/fatedier/golib/msg/json"
)

type Message = jsonMsg.Message

var msgCtl *jsonMsg.MsgCtl

func init() {
	msgCtl = jsonMsg.NewMsgCtl()
	for typeByte, msg := range msgTypeMap {
		msgCtl.RegisterMsg(typeByte, msg)
	}
}

func ReadMsgTimeout(conn net.Conn, timeout time.Duration) (msg Message, err error) {
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	msg, err = ReadMsg(conn)
	_ = conn.SetReadDeadline(time.Time{})
	return
}

func ReadMsg(c io.Reader) (msg Message, err error) {
	return msgCtl.ReadMsg(c)
}

func ReadMsgIntoTimeout(conn net.Conn, msg Message, timeout time.Duration) error {
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	if err := ReadMsgInto(conn, msg); err != nil {
		return err
	}
	_ = conn.SetReadDeadline(time.Time{})
	return nil
}

func ReadMsgInto(c io.Reader, msg Message) (err error) {
	return msgCtl.ReadMsgInto(c, msg)
}

func WriteMsg(c io.Writer, msg interface{}) (err error) {
	return msgCtl.WriteMsg(c, msg)
}
