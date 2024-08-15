package net

import (
	"bytes"
	"fmt"
	"net"
	"strings"
)

type TargetAware interface {
	GetTargetAddr() string
}

var (
	_ TargetAware = (*ConnExtra)(nil)
	_ TargetAware = (*AddrExtra)(nil)
)

func GetTarget(dst any) string {
	if target, ok := dst.(TargetAware); ok {
		return target.GetTargetAddr()
	}
	return ""
}

func WrapAddrTarget(obj any, addr net.Addr) net.Addr {
	if extra, ok := obj.(TargetAware); ok {
		return &AddrExtra{Addr: addr, TargetAddr: extra.GetTargetAddr()}
	}

	return addr
}

func WrapConnTarget(c net.Conn, targetAddr string) net.Conn {
	return &ConnExtra{
		Conn:       c,
		TargetAddr: targetAddr,
	}
}

type ConnExtra struct {
	net.Conn
	TargetAddr string
}

func (c *ConnExtra) GetTargetAddr() string { return c.TargetAddr }

type AddrExtra struct {
	net.Addr
	TargetAddr string
}

func (c *AddrExtra) GetTargetAddr() string { return c.TargetAddr }

func ParseTargetHead(userConn net.Conn) (string, error) {
	var targetBuf []byte
	buf := make([]byte, 1)
	for {
		n, err := userConn.Read(buf)
		if err != nil {
			return "", fmt.Errorf("read userConn: %w", err)
		}

		targetBuf = append(targetBuf, buf[:n]...)
		if bytes.HasSuffix(targetBuf, []byte(";")) {
			break
		}
	}

	targetAddr := string(targetBuf)
	if !strings.HasPrefix(targetAddr, "TARGET ") {
		return "", fmt.Errorf("bad head format %q", targetAddr)
	}

	targetAddr = strings.TrimPrefix(targetAddr, "TARGET ")
	targetAddr = strings.TrimSuffix(targetAddr, ";")
	return targetAddr, nil
}
