package common

import (
	"bufio"
	"net"
	"testing"
)

/*
 * Mock IRC server
 */

type MockIRCServer struct {
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

func (self *MockIRCServer) Run(t *testing.T) {

	listener, err := net.Listen("tcp", ":"+self.Port)
	if err != nil {
		t.Error("Error starting mock server on "+self.Port, err)
		return
	}

	// Accept a single connection
	conn, lerr := listener.Accept()
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
	conn.Write([]byte(":yml!~yml@li148-151.members.linode.com PRIVMSG #unit :" + self.Message + "\n"))
	//conn.Write([]byte("test: " + self.Message + "\n"))

	var derr error
	var data []byte

	bufRead := bufio.NewReader(conn)
	for {
		data, derr = bufRead.ReadBytes('\n')
		if derr != nil {
			// Client closed connection
			break
		}
		self.Got = append(self.Got, string(data))
	}

}
