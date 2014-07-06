package common

import (
	"bufio"
	"crypto/tls"
	"flag"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/golang/glog"
)

// SetGlogFlags walk around a glog issue and force it to log to stderr.
// It need to be called at the beginning of each test.
func SetGlogFlags() {
	flag.Set("alsologtostderr", "true")
	flag.Set("v", "3")
}

// MockSocket is a dummy implementation of ReadWriteCloser
type MockSocket struct {
	sync.RWMutex
	Counter  chan bool
	Receiver chan string
}

func (sock MockSocket) Write(data []byte) (int, error) {
	glog.V(3).Infoln("[Debug]: Starting MockSocket.Write of:", string(data))
	if sock.Counter != nil {
		sock.Counter <- true
	}
	if sock.Receiver != nil {
		sock.Receiver <- string(data)
	}

	return len(data), nil
}

func (sock MockSocket) Read(into []byte) (int, error) {
	sock.RLock()
	defer sock.RUnlock()
	time.Sleep(time.Second) // Prevent busy loop
	return 0, nil
}

func (sock MockSocket) Close() error {
	if sock.Receiver != nil {
		close(sock.Receiver)
	}
	if sock.Counter != nil {
		close(sock.Counter)
	}
	return nil
}

/*
 * Mock IRC server
 */

type MockIRCServer struct {
	sync.RWMutex
	Port    string
	Message string
	Got     []string
}

func NewMockIRCServer(msg, port string) *MockIRCServer {
	return &MockIRCServer{
		Port:    port,
		Message: msg,
		Got:     make([]string, 0),
	}
}

func (srv *MockIRCServer) GotLength() int {
	srv.RLock()
	defer srv.RUnlock()
	return len(srv.Got)
}

func (srv *MockIRCServer) Run(t *testing.T) {
	// Use the certs generated with generate_certs
	cert, err := tls.LoadX509KeyPair("certs/cert.pem", "certs/key.pem")
	if err != nil {
		log.Fatalf("server: loadkeys: %s", err)
	}
	config := tls.Config{Certificates: []tls.Certificate{cert}}
	listener, err := tls.Listen("tcp", "127.0.0.1:"+srv.Port, &config)
	if err != nil {
		t.Error("Error starting mock server on "+srv.Port, err)
		return
	}

	for {
		conn, lerr := listener.Accept()
		// If create a new connection throw the old data away
		// This can happen if a client trys to connect with tls
		// Got will store the handshake data. The cient will try
		// connect with a plaintext connect after the tls fails.
		srv.Lock()
		srv.Got = make([]string, 0)
		srv.Unlock()

		if lerr != nil {
			t.Error("Error on IRC server on Accept. ", err)
		}

		// First message triggers BotBot to send USER and NICK messages
		conn.Write([]byte(":hybrid7.debian.local NOTICE AUTH :*** Looking up your hostname...\n"))
		// Ask for NickServ auth, and pretend we got it
		conn.Write([]byte(":NickServ!NickServ@services. NOTICE graham_king :This nickname is registered. Please choose a different nickname, or identify via /msg NickServ identify <password>\n"))
		conn.Write([]byte(":NickServ!NickServ@services. NOTICE graham_king :You are now identified for graham_king.\n"))
		conn.Write([]byte(":wolfe.freenode.net 001 graham_king :Welcome to the freenode Internet Relay Chat Network graham_king\n"))
		// This should get sent to plugins
		conn.Write([]byte(":yml!~yml@li148-151.members.linode.com PRIVMSG #unit :" + srv.Message + "\n"))
		conn.Write([]byte("test: " + srv.Message + "\n"))

		var derr error
		var data []byte

		bufRead := bufio.NewReader(conn)
		for {
			data, derr = bufRead.ReadBytes('\n')
			if derr != nil {
				// Client closed connection
				break
			}
			srv.Lock()
			srv.Got = append(srv.Got, string(data))
			srv.Unlock()
		}
	}

}
