package main

import (
	"flag"
	"github.com/gorilla/websocket"
	"github.com/nsecgo/wstun/socks5"
	"github.com/xtaci/smux"
	"io"
	"log"
	"net"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	var url = flag.String("url", "wss://gg.gg/password", "webSocket server url")
	var l = flag.String("l", "127.0.0.1:1080", "listen address")
	var ipv4 = flag.Bool("4", false, "ipv4 only")
	flag.Parse()

	if *ipv4 {
		websocket.DefaultDialer.NetDial = func(network, addr string) (conn net.Conn, e error) {
			return net.Dial("tcp4", addr)
		}
	}

	createSession := func() *smux.Session {
		session, err := createSessionBaseWs(*url)
		if err != nil {
			log.Fatal("[ERROR] createSessionBaseWs:", err)
		}
		log.Println("Successfully established a session")
		return session
	}
	session := createSession()

	tcpLn, err := net.Listen("tcp", *l)
	if err != nil {
		log.Fatal("[ERROR] listen:", err)
	}
	log.Println("listen on ", *l)

	for {
		lConn, err := tcpLn.Accept()
		if err != nil {
			log.Fatal("[ERROR] TCP accept: ", err)
		}
		reqAddr, cmd := socks5.Handshake(lConn)
		if cmd != socks5.CmdConnect {
			lConn.Close()
			continue
		}
	openStream:
		stream, err := session.OpenStream()
		if err != nil {
			log.Println("[ERROR] openStream:", err)
			if session.IsClosed() {
				session = createSession()
				goto openStream
			} else {
				lConn.Close()
				continue
			}
		}
		go func() {
			defer func() {
				lConn.Close()
				stream.Close()
			}()
			if cmd == socks5.CmdConnect {
				_, err = stream.Write(reqAddr)
				if err != nil {
					return
				}
				go io.Copy(lConn, stream)
				io.Copy(stream, lConn)
			}
			// TODO: Support for the UDP ASSOCIATE command
		}()
	}
}
func createSessionBaseWs(url string) (*smux.Session, error) {
	wsConn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, err
	}
	session, err := smux.Client(wsConn.UnderlyingConn(), nil)
	if err != nil {
		return nil, err
	}
	return session, nil
}
