package socks5

import (
	"bytes"
	"io"
	"net"
	"strconv"
)

var ConnectResp = []byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}
var UDPResp = UDPResponse("127.0.0.1", 1080)

const MaxAddrLen = 1 + 1 + 255 + 2
const (
	CmdConnect      = 1
	CmdBind         = 2
	CmdUDPAssociate = 3
)
const (
	ATYPIPv4       = 1
	ATYPDomainName = 3
	ATYPIPv6       = 4
)

// Addr represents a SOCKS address as defined in RFC 1928 section 5.
type ReqAddr []byte

// String serializes SOCKS address a to string form.
func (a ReqAddr) String() string {
	var host, port string
	n := len(a)
	switch a[0] { // address type
	case ATYPDomainName:
		host = string(a[2 : n-2])
		port = strconv.Itoa((int(a[n-2]) << 8) | int(a[n-1]))
	case ATYPIPv4:
		host = net.IP(a[1 : 1+net.IPv4len]).String()
		port = strconv.Itoa((int(a[n-2]) << 8) | int(a[n]-1))
	case ATYPIPv6:
		host = net.IP(a[1 : 1+net.IPv6len]).String()
		port = strconv.Itoa((int(a[n-2]) << 8) | int(a[n-1]))
	}

	return net.JoinHostPort(host, port)
}
func (a ReqAddr) validate() bool {
	n := len(a)
	switch a[0] {
	case ATYPDomainName:
		if n-2-2 == int(a[1]) {
			return true
		}
	case ATYPIPv4:
		if n == 1+net.IPv4len+2 {
			return true
		}
	case ATYPIPv6:
		if n == 1+net.IPv6len+2 {
			return true
		}
	}
	return false
}
func ReadAddr(r io.Reader) ReqAddr {
	buf := make([]byte, MaxAddrLen)
	_, err := r.Read(buf[:1]) // read 1st byte for address type
	if err != nil {
		return nil
	}
	switch buf[0] {
	case ATYPDomainName:
		_, err = r.Read(buf[1:2]) // read 2nd byte for domain length
		if err != nil {
			return nil
		}
		_, err = io.ReadFull(r, buf[2:2+int(buf[1])+2])
		if err != nil {
			return nil
		}
		return buf[:1+1+int(buf[1])+2]
	case ATYPIPv4:
		_, err = io.ReadFull(r, buf[1:1+net.IPv4len+2])
		if err != nil {
			return nil
		}
		return buf[:1+net.IPv4len+2]
	case ATYPIPv6:
		_, err = io.ReadFull(r, buf[1:1+net.IPv6len+2])
		if err != nil {
			return nil
		}
		return buf[:1+net.IPv6len+2]
	}
	return nil
}

// SplitAddr slices a SOCKS address from beginning of b. Returns nil if failed.
func SplitAddr(b []byte) ReqAddr {
	addrLen := 1
	if len(b) < addrLen {
		return nil
	}

	switch b[0] {
	case ATYPDomainName:
		if len(b) < 2 {
			return nil
		}
		addrLen = 1 + 1 + int(b[1]) + 2
	case ATYPIPv4:
		addrLen = 1 + net.IPv4len + 2
	case ATYPIPv6:
		addrLen = 1 + net.IPv6len + 2
	default:
		return nil

	}

	if len(b) < addrLen {
		return nil
	}

	return b[:addrLen]
}
func UDPResponse(host string, port uint16) []byte {
	return append([]byte{5, 0, 0}, ParseAddr(host, port)...)
}

// ParseAddr parses the address in string s. Returns nil if failed.
func ParseAddr(host string, port uint16) ReqAddr {
	var addr ReqAddr
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			addr = make([]byte, 1+net.IPv4len+2)
			addr[0] = ATYPIPv4
			copy(addr[1:], ip4)
		} else {
			addr = make([]byte, 1+net.IPv6len+2)
			addr[0] = ATYPIPv6
			copy(addr[1:], ip)
		}
	} else {
		if len(host) > 255 {
			return nil
		}
		addr = make([]byte, 1+1+len(host)+2)
		addr[0] = ATYPDomainName
		addr[1] = byte(len(host))
		copy(addr[2:], host)
	}
	addr[len(addr)-2], addr[len(addr)-1] = byte(port>>8), byte(port)
	return addr
}

func Handshake(rw io.ReadWriter) (ReqAddr, byte) {
	buf := make([]byte, MaxAddrLen)
	/*
		+----+----------+----------+
		|VER | NMETHODS | METHODS  |
		+----+----------+----------+
		| 1  |    1     | 1 to 255 |
		+----+----------+----------+
	*/
	n, err := rw.Read(buf)
	if err != nil || n != 3 || !bytes.Equal(buf[:3], []byte{5, 1, 0}) {
		return nil, 0
	}
	/*
		+----+--------+
		|VER | METHOD |
		+----+--------+
		| 1  |   1    |
		+----+--------+
	*/
	_, err = rw.Write([]byte{5, 0})
	if err != nil {
		return nil, 0
	}
	/*
		+----+-----+-------+------+----------+----------+
		|VER | CMD |  RSV  | ATYP | DST.ADDR | DST.PORT |
		+----+-----+-------+------+----------+----------+
		| 1  |  1  | X'00' |  1   | Variable |    2     |
		+----+-----+-------+------+----------+----------+
	*/
	n, err = rw.Read(buf[:3])
	if err != nil {
		return nil, 0
	}
	cmd := buf[1]
	n, err = rw.Read(buf)
	if err != nil {
		return nil, 0
	}
	var reqAddr ReqAddr = buf[:n]
	if !reqAddr.validate() {
		return nil, 0
	}
	/*
		    +----+-----+-------+------+----------+----------+
	        |VER | REP |  RSV  | ATYP | BND.ADDR | BND.PORT |
	        +----+-----+-------+------+----------+----------+
	        | 1  |  1  | X'00' |  1   | Variable |    2     |
	        +----+-----+-------+------+----------+----------+
	*/
	switch cmd {
	case CmdConnect:
		_, err = rw.Write(ConnectResp)
	case CmdUDPAssociate:
		_, err = rw.Write(UDPResp)
	default:
		return nil, 0
	}
	if err != nil {
		return nil, 0
	}
	return reqAddr, cmd
}
