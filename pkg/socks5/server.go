package socks5

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
)

// Server is accepting connections and handling the details of the SOCKS5 protocol
type Server struct {
	// Authentication is proxy authentication
	Authentication Authentication
	// ProxyDial specifies the optional proxyDial function for
	// establishing the transport connection.
	ProxyDial func(ctx context.Context, network, address string) (net.Conn, error)
	// ProxyListen specifies the optional proxyListen function for
	// establishing the transport connection.
	ProxyListen func(context.Context, string, string) (net.Listener, error)
	// ProxyListenPacket specifies the optional proxyListenPacket function for
	// establishing the transport connection.
	ProxyListenPacket func(ctx context.Context, network, address string) (net.PacketConn, error)
	// PacketForwardAddress specifies the packet forwarding address
	PacketForwardAddress func(ctx context.Context, destinationAddr string, packet net.PacketConn, conn net.Conn) (net.IP, int, error)
	// Logger error log
	Logger Logger
	// Context is default context
	Context context.Context
	// BytesPool getting and returning temporary bytes for use by io.CopyBuffer
	BytesPool BytesPool
}

type Logger interface {
	Println(v ...any)
}

// NewServer creates a new Server
func NewServer() *Server {
	return &Server{}
}

// ListenAndServe is used to create a listener and serve on it
func (s *Server) ListenAndServe(network, addr string) error {
	l, err := s.proxyListen(s.context(), network, addr)
	if err != nil {
		return err
	}
	return s.Serve(l)
}

func (s *Server) proxyListen(ctx context.Context, network, address string) (net.Listener, error) {
	proxyListen := s.ProxyListen
	if proxyListen == nil {
		proxyListen = (&net.ListenConfig{}).Listen
	}
	return proxyListen(ctx, network, address)
}

// Serve is used to serve connections from a listener
func (s *Server) Serve(l net.Listener) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		go s.ServeConn(conn)
	}
}

// ServeConn is used to serve a single connection.
func (s *Server) ServeConn(conn net.Conn) {
	defer conn.Close()
	err := s.serveConn(conn)
	if err != nil && s.Logger != nil && !isClosedConnError(err) {
		s.Logger.Println(err)
	}
}

func (s *Server) serveConn(conn net.Conn) error {
	req, err := s.ParseRequest(conn)
	if err != nil {
		return err
	}

	if err := s.handle(req); err != nil {
		return err
	}

	return nil
}

func (s *Server) ParseRequest(conn net.Conn) (*Request, error) {
	version, err := readByte(conn)
	if err != nil {
		return nil, err
	}
	if version != socks5Version {
		return nil, fmt.Errorf("unsupported SOCKS version: %d", version)
	}

	req := &Request{
		Version: socks5Version,
		Conn:    conn,
	}

	methods, err := readBytes(conn)
	if err != nil {
		return nil, err
	}

	if s.Authentication != nil && bytes.IndexByte(methods, byte(userAuth)) != -1 {
		if _, err := conn.Write([]byte{socks5Version, byte(userAuth)}); err != nil {
			return nil, err
		}

		header, err := readByte(conn)
		if err != nil {
			return nil, err
		}
		if header != userAuthVersion {
			return nil, fmt.Errorf("unsupported auth version: %d", header)
		}

		username, err := readBytes(conn)
		if err != nil {
			return nil, err
		}
		req.Username = string(username)

		password, err := readBytes(conn)
		if err != nil {
			return nil, err
		}
		req.Password = string(password)

		if !s.Authentication.Auth(req.Command, req.Username, req.Password) {
			_, err := conn.Write([]byte{userAuthVersion, authFailure})
			if err != nil {
				return nil, err
			}
			return nil, errUserAuthFailed
		}
		if _, err := conn.Write([]byte{userAuthVersion, authSuccess}); err != nil {
			return nil, err
		}
	} else if s.Authentication == nil && bytes.IndexByte(methods, byte(noAuth)) != -1 {
		if _, err := conn.Write([]byte{socks5Version, byte(noAuth)}); err != nil {
			return nil, err
		}
	} else {
		if _, err := conn.Write([]byte{socks5Version, byte(noAcceptable)}); err != nil {
			return nil, err
		}
		return nil, errNoSupportedAuth
	}

	var header [3]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return nil, err
	}

	if header[0] != socks5Version {
		return nil, fmt.Errorf("unsupported Command version: %d", header[0])
	}

	req.Command = Command(header[1])

	dest, err := readAddr(conn)
	if err != nil {
		if errors.Is(err, errUnrecognizedAddrType) {
			if err := sendReply(conn, addrTypeNotSupported, nil); err != nil {
				return nil, err
			}
		}
		return nil, err
	}
	req.DestinationAddr = dest

	return req, nil

}

func (s *Server) handle(req *Request) error {
	switch req.Command {
	case ConnectCommand:
		return s.handleConnect(req)
	case BindCommand:
		return s.handleBind(req)
	case AssociateCommand:
		return s.handleAssociate(req)
	default:
		if err := sendReply(req.Conn, commandNotSupported, nil); err != nil {
			return err
		}
		return fmt.Errorf("unsupported Command: %v", req.Command)
	}
}

func (s *Server) handleConnect(req *Request) error {
	ctx := s.context()
	target, err := s.proxyDial(ctx, "tcp", req.DestinationAddr.Address())
	if err != nil {
		if err := sendReply(req.Conn, errToReply(err), nil); err != nil {
			return fmt.Errorf("failed to send reply: %v", err)
		}
		return fmt.Errorf("connect to %v failed: %w", req.DestinationAddr, err)
	}
	defer target.Close()

	localAddr := target.LocalAddr()
	_, port, err := net.SplitHostPort(localAddr.String())
	if err != nil {
		return fmt.Errorf("connect to %v failed: local address is %s://%s", req.DestinationAddr, localAddr.Network(), localAddr)
	}
	intPort, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("connect to %v failed: local address is %s://%s", req.DestinationAddr, localAddr.Network(), localAddr)
	}
	bind := address{IP: net.ParseIP("0.0.0.0"), Port: intPort}
	if err := sendReply(req.Conn, successReply, &bind); err != nil {
		return fmt.Errorf("failed to send reply: %v", err)
	}

	var buf1, buf2 []byte
	if s.BytesPool != nil {
		buf1 = s.BytesPool.Get()
		buf2 = s.BytesPool.Get()
		defer func() {
			s.BytesPool.Put(buf1)
			s.BytesPool.Put(buf2)
		}()
	} else {
		buf1 = make([]byte, 32*1024)
		buf2 = make([]byte, 32*1024)
	}
	return tunnel(ctx, target, req.Conn, buf1, buf2)
}

func SendSuccessReply(req, target net.Conn) error {
	ipPort := target.LocalAddr().String()
	_, port, err := net.SplitHostPort(ipPort)
	if err != nil {
		return fmt.Errorf("failed to split host port: %v", err)
	}

	intPort, _ := strconv.Atoi(port)
	bind := address{IP: net.ParseIP("127.0.0.1"), Port: intPort}
	if err := sendReply(req, successReply, &bind); err != nil {
		return fmt.Errorf("failed to send reply: %v", err)
	}
	return nil
}

func (s *Server) handleBind(req *Request) error {
	ctx := s.context()

	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", req.DestinationAddr.String())
	if err != nil {
		if err := sendReply(req.Conn, errToReply(err), nil); err != nil {
			return fmt.Errorf("failed to send reply: %v", err)
		}
		return fmt.Errorf("connect to %v failed: %w", req.DestinationAddr, err)
	}

	localAddr := listener.Addr()
	local, ok := localAddr.(*net.TCPAddr)
	if !ok {
		listener.Close()
		return fmt.Errorf("connect to %v failed: local address is %s://%s", req.DestinationAddr, localAddr.Network(), localAddr.String())
	}
	bind := address{IP: local.IP, Port: local.Port}
	if err := sendReply(req.Conn, successReply, &bind); err != nil {
		listener.Close()
		return fmt.Errorf("failed to send reply: %v", err)
	}

	conn, err := listener.Accept()
	if err != nil {
		listener.Close()
		if err := sendReply(req.Conn, errToReply(err), nil); err != nil {
			return fmt.Errorf("failed to send reply: %v", err)
		}
		return fmt.Errorf("connect to %v failed: %w", req.DestinationAddr, err)
	}
	listener.Close()

	remoteAddr := conn.RemoteAddr()
	local, ok = remoteAddr.(*net.TCPAddr)
	if !ok {
		return fmt.Errorf("connect to %v failed: remote address is %s://%s", req.DestinationAddr, localAddr.Network(), localAddr.String())
	}
	bind = address{IP: local.IP, Port: local.Port}
	if err := sendReply(req.Conn, successReply, &bind); err != nil {
		return fmt.Errorf("failed to send reply: %v", err)
	}

	var buf1, buf2 []byte
	if s.BytesPool != nil {
		buf1 = s.BytesPool.Get()
		buf2 = s.BytesPool.Get()
		defer func() {
			s.BytesPool.Put(buf1)
			s.BytesPool.Put(buf2)
		}()
	} else {
		buf1 = make([]byte, 32*1024)
		buf2 = make([]byte, 32*1024)
	}
	return tunnel(ctx, conn, req.Conn, buf1, buf2)
}

func (s *Server) handleAssociate(req *Request) error {
	ctx := s.context()
	destinationAddr := req.DestinationAddr.String()
	udpConn, err := s.proxyListenPacket(ctx, "udp", destinationAddr)
	if err != nil {
		if err := sendReply(req.Conn, errToReply(err), nil); err != nil {
			return fmt.Errorf("failed to send reply: %v", err)
		}
		return fmt.Errorf("connect to %v failed: %w", req.DestinationAddr, err)
	}
	defer udpConn.Close()

	replyPacketForwardAddress := defaultReplyPacketForwardAddress
	if s.PacketForwardAddress != nil {
		replyPacketForwardAddress = s.PacketForwardAddress
	}
	ip, port, err := replyPacketForwardAddress(ctx, destinationAddr, udpConn, req.Conn)
	if err != nil {
		return err
	}
	bind := address{IP: ip, Port: port}
	if err := sendReply(req.Conn, successReply, &bind); err != nil {
		return fmt.Errorf("failed to send reply: %v", err)
	}

	go func() {
		var buf [1]byte
		for {
			_, err := req.Conn.Read(buf[:])
			if err != nil {
				udpConn.Close()
				break
			}
		}
	}()

	var (
		sourceAddr  net.Addr
		wantSource  string
		targetAddr  net.Addr
		wantTarget  string
		replyPrefix []byte
		buf         [maxUdpPacket]byte
	)

	for {
		n, addr, err := udpConn.ReadFrom(buf[:])
		if err != nil {
			return err
		}

		if sourceAddr == nil {
			sourceAddr = addr
			wantSource = sourceAddr.String()
		}

		gotAddr := addr.String()
		if wantSource == gotAddr {
			if n < 3 {
				continue
			}
			reader := bytes.NewBuffer(buf[3:n])
			addr, err := readAddr(reader)
			if err != nil {
				if s.Logger != nil {
					s.Logger.Println(err)
				}
				continue
			}
			if targetAddr == nil {
				targetAddr = &net.UDPAddr{
					IP:   addr.IP,
					Port: addr.Port,
				}
				wantTarget = targetAddr.String()
			}
			if addr.String() != wantTarget {
				if s.Logger != nil {
					s.Logger.Println(fmt.Errorf("ignore non-target addresses %s", addr))
				}
				continue
			}
			_, err = udpConn.WriteTo(reader.Bytes(), targetAddr)
			if err != nil {
				return err
			}
		} else if targetAddr != nil && wantTarget == gotAddr {
			if replyPrefix == nil {
				b := bytes.NewBuffer(make([]byte, 3, 16))
				err = writeAddrWithStr(b, wantTarget)
				if err != nil {
					return err
				}
				replyPrefix = b.Bytes()
			}
			copy(buf[len(replyPrefix):len(replyPrefix)+n], buf[:n])
			copy(buf[:len(replyPrefix)], replyPrefix)
			_, err = udpConn.WriteTo(buf[:len(replyPrefix)+n], sourceAddr)
			if err != nil {
				return err
			}
		}
	}
}

func (s *Server) proxyDial(ctx context.Context, network, address string) (net.Conn, error) {
	proxyDial := s.ProxyDial
	if proxyDial == nil {
		var dialer net.Dialer
		proxyDial = dialer.DialContext
	}
	return proxyDial(ctx, network, address)
}

func (s *Server) proxyListenPacket(ctx context.Context, network, address string) (net.PacketConn, error) {
	proxyListenPacket := s.ProxyListenPacket
	if proxyListenPacket == nil {
		var listener net.ListenConfig
		proxyListenPacket = listener.ListenPacket
	}
	return proxyListenPacket(ctx, network, address)
}

func (s *Server) context() context.Context {
	if s.Context == nil {
		return context.Background()
	}
	return s.Context
}

func sendReply(w io.Writer, resp reply, addr *address) error {
	_, err := w.Write([]byte{socks5Version, byte(resp), 0})
	if err != nil {
		return err
	}
	err = writeAddr(w, addr)
	return err
}

type Request struct {
	Version         uint8
	Command         Command
	DestinationAddr *address
	Username        string
	Password        string
	Conn            net.Conn
}

func defaultReplyPacketForwardAddress(ctx context.Context, destinationAddr string, packet net.PacketConn, conn net.Conn) (net.IP, int, error) {
	udpLocal := packet.LocalAddr()
	udpLocalAddr, ok := udpLocal.(*net.UDPAddr)
	if !ok {
		return nil, 0, fmt.Errorf("connect to %v failed: local address is %s://%s", destinationAddr, udpLocal.Network(), udpLocal.String())
	}

	tcpLocal := conn.LocalAddr()
	tcpLocalAddr, ok := tcpLocal.(*net.TCPAddr)
	if !ok {
		return nil, 0, fmt.Errorf("connect to %v failed: local address is %s://%s", destinationAddr, tcpLocal.Network(), tcpLocal.String())
	}
	return tcpLocalAddr.IP, udpLocalAddr.Port, nil
}
