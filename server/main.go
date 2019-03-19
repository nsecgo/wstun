package main

import (
	"flag"
	"github.com/gorilla/websocket"
	"github.com/nsecgo/wstun/socks5"
	"github.com/xtaci/smux"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	var pwd = flag.String("pwd", "password", "password")
	var addr = flag.String("l", ":443", "listen address")
	var certFile = flag.String("cert", "", "cert file")
	var keyFile = flag.String("key", "", "key file")
	var fileServerPath = flag.String("fp", "", "file server path")
	flag.Parse()

	var upgrade = websocket.Upgrader{} // use default options
	http.HandleFunc("/"+*pwd, func(w http.ResponseWriter, r *http.Request) {
		wc, err := upgrade.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer wc.Close()
		log.Println("New connection:", wc.RemoteAddr())
		myLog := log.New(os.Stderr, "["+wc.RemoteAddr().String()+"] ", log.LstdFlags|log.Lshortfile)

		session, err := smux.Server(wc.UnderlyingConn(), nil)
		if err != nil {
			myLog.Println("[ERROR] create mux server:", err)
			return
		}
		myLog.Println("Successfully established a session")
		for {
			stream, err := session.AcceptStream()
			if err != nil {
				myLog.Println("[ERROR] accept:", err)
				return
			}
			addr := socks5.ReadAddr(stream)
			if addr == nil {
				stream.Close()
				continue
			}
			go func() {
				c, err := net.Dial("tcp", addr.String())
				if err != nil {
					myLog.Println("real server dial:", err, "[Closing stream]", stream.Close())
					return
				}
				defer func() {
					stream.Close()
					c.Close()
				}()
				go io.Copy(stream, c)
				io.Copy(c, stream)
			}()
		}
	})
	if *fileServerPath != "" {
		http.Handle("/files/", http.StripPrefix("/files/", http.FileServer(http.Dir(*fileServerPath))))
	}
	server := http.Server{Addr: *addr, ErrorLog: log.New(ioutil.Discard, "", log.LstdFlags)}
	if *certFile == "" {
		log.Fatal(server.ListenAndServe())
	} else {
		log.Fatal(server.ListenAndServeTLS(*certFile, *keyFile))
	}
}
