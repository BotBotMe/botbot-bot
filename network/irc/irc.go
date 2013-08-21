// IRC server connection
//
// Connecting to an IRC server goes like this:
// 1. Connect to the socket. Wait for a response (anything will do).
// 2. Send USER and NICK. Wait for a response (anything).
// 2.5 If we have a password, wait for NickServ to ask for it, and to confirm authentication
// 3. JOIN channels

package irc

import (
	"github.com/lincolnloop/botbot-bot/common"
	"github.com/lincolnloop/botbot-bot/line"
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"io"
	"log"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	VERSION = "botbotme v0.2"
)

type ircBot struct {
	id               int
	address          string
	socket           io.ReadWriteCloser
	nick             string
	realname         string
	password         string
	serverPass       string
	fromServer       chan *line.Line
	channels         []string
	isRunning        bool
	isConnecting     bool
	isAuthenticating bool
	sendQueue        chan []byte
	monitorChan      chan string
}

func NewBot(config *common.BotConfig, fromServer chan *line.Line) common.ChatBot {

	// realname is set to config["realname"] or config["nick"]
	realname := config.Config["realname"]
	if realname == "" {
		realname = config.Config["nick"]
	}

	chatbot := &ircBot{
		id:          config.Id,
		address:     config.Config["server"],
		nick:        config.Config["nick"],
		realname:    realname,
		password:    config.Config["password"],        // NickServ password
		serverPass:  config.Config["server_password"], // PASS password
		fromServer:  fromServer,
		channels:    config.Channels,
		monitorChan: make(chan string),
		isRunning:   true,
	}

	chatbot.init()

	return chatbot
}
func (self *ircBot) GetUser() string {
	return self.nick
}

// Monitor that we are still connected to the IRC server
// should run in go-routine
func (self *ircBot) monitor() {
	for self.isRunning {
		select {
		case <-self.monitorChan:
		case <-time.After(time.Minute * 10):
			log.Println("IRC monitoring KO ; Trying to reconnect.")
			self.Close()
			self.Connect()
		}
	}
}

// Ping the server every 5 min to keep the connection open
func (self *ircBot) pinger() {
	i := 0
	for self.isRunning {
		<-time.After(time.Minute * 5)
		i = i + 1
		self.SendRaw("PING " + strconv.Itoa(i))
	}
}

// Connect to the IRC server and start listener
func (self *ircBot) init() {

	self.isConnecting = true
	self.isAuthenticating = false

	self.Connect()

	// Listen for incoming messages in background thread
	go self.listen()

	// Monitor that we are still getting incoming messages in a background thread
	go self.monitor()

	// Pinger that Ping the server every 5 min
	go self.pinger()

	// Listen for outgoing messages (rate limited) in background thread
	if self.sendQueue == nil {
		self.sendQueue = make(chan []byte, 256)
	}
	go self.sender()

	if self.serverPass != "" {
		self.SendRaw("PASS " + self.serverPass)
	}

	self.SendRaw("PING Bonjour")
}

// Connect to the server
func (self *ircBot) Connect() {

	if self.socket != nil {
		// socket already set, unit tests need this
		return
	}

	var socket net.Conn
	var err error

	for {
		log.Println("Connecting to IRC server: ", self.address)

		socket, err = tls.Dial("tcp", self.address, nil) // Always try TLS first
		if err == nil {
			log.Println("Connected: TLS secure")
			break
		}

		log.Println("Could not connect using TLS because: ", err)

		_, ok := err.(x509.HostnameError)
		if ok {
			// Certificate might not match. This happens on irc.cloudfront.net
			insecure := &tls.Config{InsecureSkipVerify: true}
			socket, err = tls.Dial("tcp", self.address, insecure)

			if err == nil && isCertValid(socket.(*tls.Conn)) {
				log.Println("Connected: TLS with awkward certificate")
				break
			}
		}

		socket, err = net.Dial("tcp", self.address)

		if err == nil {
			log.Println("Connected: Plain text insecure")
			break
		}

		log.Println("IRC Connect error. Will attempt to re-connect. ", err)
		time.Sleep(1 * time.Second)
	}

	self.socket = socket
}

/* Check that the TLS connection's certficate can be applied to this connection.
Because irc.coldfront.net presents a certificate not as irc.coldfront.net, but as it's actual host (e.g. snow.coldfront.net),

We do this by comparing the IP address of the certs name to the IP address of our connection.
If they match we're OK.
*/
func isCertValid(conn *tls.Conn) bool {
	connAddr := strings.Split(conn.RemoteAddr().String(), ":")[0]
	cert := conn.ConnectionState().PeerCertificates[0]

	if len(cert.DNSNames) == 0 {
		// Cert has single name, the usual case
		return isIPMatch(cert.Subject.CommonName, connAddr)

	} else {
		// Cert has several valid names
		for _, certname := range cert.DNSNames {
			if isIPMatch(certname, connAddr) {
				return true
			}
		}
	}

	return false
}

// Does hostname have IP address connIP?
func isIPMatch(hostname string, connIP string) bool {
	log.Println("Checking IP of", hostname)

	addrs, err := net.LookupIP(hostname)
	if err != nil {
		log.Println("Error DNS lookup of "+hostname+": ", err)
		return false
	}

	for _, ip := range addrs {
		if ip.String() == connIP {
			log.Println("Accepting certificate anyway. " + hostname + " has same IP as connection")
			return true
		}
	}
	return false
}

// Update bot configuration. Called when webapp changes a chatbot's config.
func (self *ircBot) Update(config *common.BotConfig) {

	isNewServer := self.updateServer(config.Config)
	if isNewServer {
		// If the server changed, we've already done nick and channel changes too
		return
	}

	self.updateNick(config.Config["nick"], config.Config["password"])
	self.updateChannels(config.Channels)
}

// Update the IRC server we're connected to
func (self *ircBot) updateServer(config map[string]string) bool {

	addr := config["server"]
	if addr == self.address {
		return false
	}

	log.Println("Changing IRC server from ", self.address, " to ", addr)

	self.Close()
	time.Sleep(1 * time.Second) // Wait for timeout to be sure listen has stopped

	self.address = addr
	self.nick = config["nick"]
	self.password = config["password"]
	self.channels = splitChannels(config["rooms"])

	self.init()

	return true
}

// Update the nickname we're registered under, if needed
func (self *ircBot) updateNick(newNick, newPass string) {
	if newNick == self.nick {
		return
	}

	self.nick = newNick
	self.password = newPass
	self.setNick()
}

// Update the channels based on new configuration, leaving old ones and joining new ones
func (self *ircBot) updateChannels(newChannels []string) {

	if isEqual(newChannels, self.channels) {
		return
	}

	// PART old ones
	for _, channel := range self.channels {
		if !isIn(channel, newChannels) {
			self.part(channel)
		}
	}

	// JOIN new ones
	for _, channel := range newChannels {
		if !isIn(channel, self.channels) {
			self.join(channel)
		}
	}

	self.channels = newChannels
}

// Join channels
func (self *ircBot) JoinAll() {
	for _, channel := range self.channels {
		self.join(channel)
	}
}

// Join an IRC channel
func (self *ircBot) join(channel string) {
	self.SendRaw("JOIN " + channel)
}

// Leave an IRC channel
func (self *ircBot) part(channel string) {
	self.SendRaw("PART " + channel)
}

// Send a regular (non-system command) IRC message
func (self *ircBot) Send(channel, msg string) {
	fullmsg := "PRIVMSG " + channel + " :" + msg
	self.SendRaw(fullmsg)
}

// Send message down socket. Add \n at end first.
func (self *ircBot) SendRaw(msg string) {
	self.sendQueue <- []byte(msg + "\n")
}

// Tell the irc server who we are - we can't do anything until this is done.
func (self *ircBot) login() {

	self.isAuthenticating = true

	// We use the botname as the 'realname', because bot's don't have real names!
	self.SendRaw("USER " + self.nick + " 0 * :" + self.realname)

	self.setNick()
}

// Tell the network our nick
func (self *ircBot) setNick() {
	self.SendRaw("NICK " + self.nick)
}

// Tell NickServ our password
func (self *ircBot) sendPassword() {
	self.Send("NickServ", "identify "+self.password)
}

// Actually really send message to the server. Implements rate limiting.
// Should run in go-routine.
func (self *ircBot) sender() {

	var data []byte
	var twoSeconds = time.Second * 2
	var err error

	for self.isRunning {

		data = <-self.sendQueue
		log.Print("[RAW"+strconv.Itoa(self.id)+"] -->", string(data))

		_, err = self.socket.Write(data)
		if err != nil {
			self.isRunning = false
			log.Println("Error writing to socket", err)
			log.Println("Stopping chatbot. Monitor can restart it.")
			self.Close()
		}

		// Rate limit to one message every 2 seconds
		// https://github.com/lincolnloop/botbot-bot/issues/2
		time.Sleep(twoSeconds)
	}
}

// Listen for incoming messages. Parse them and put on channel.
// Should run in go-routine
func (self *ircBot) listen() {

	var contentData []byte
	var content string
	var err error

	bufRead := bufio.NewReader(self.socket)
	for self.isRunning {
		contentData, err = bufRead.ReadBytes('\n')

		if err != nil {
			netErr, ok := err.(net.Error)
			if ok && netErr.Timeout() == true {
				continue

			} else if !self.isRunning {
				// Close() wants us to stop
				return

			} else {
				log.Println("Lost IRC server connection. ", err)
				self.Close()
				return
			}
		}

		if len(contentData) == 0 {
			continue
		}

		content = toUnicode(contentData)

		log.Print("[RAW" + strconv.Itoa(self.id) + "]" + content)

		theLine, err := parseLine(content)
		if err == nil {
			theLine.ChatBotId = self.id
			self.act(theLine)
		} else {
			log.Println("Invalid line:", content)
		}

	}
}

func (self *ircBot) act(theLine *line.Line) {

	// Send the command on the monitorChan
	self.monitorChan <- theLine.Command

	// As soon as we receive a message from the server, complete initiatization
	if self.isConnecting {
		self.isConnecting = false
		self.login()
		return
	}

	// NickServ interactions

	isNickServ := strings.Contains(theLine.User, "NickServ")

	// freenode, coldfront
	isAskingForPW := strings.Contains(theLine.Content, "This nickname is registered")
	// irc.mozilla.org - and probably others, they often remind us how to identify
	isAskingForPW = isAskingForPW || strings.Contains(theLine.Content, "NickServ IDENTIFY")

	// freenode
	isConfirm := strings.Contains(theLine.Content, "You are now identified")
	// irc.mozilla.org, coldfront
	isConfirm = isConfirm || strings.Contains(theLine.Content, "you are now recognized")

	if isNickServ {

		if isAskingForPW {
			self.sendPassword()
			return

		} else if isConfirm {
			self.isAuthenticating = false
			self.JoinAll()
			return
		}
	}

	// After USER / NICK is accepted, join all the channels,
	// assuming we don't need to identify with NickServ

	if self.isAuthenticating && len(self.password) == 0 {
		self.isAuthenticating = false
		self.JoinAll()
		return
	}

	if theLine.Command == "PING" {
		// Reply, and send message on to client
		self.SendRaw("PONG " + theLine.Content)
	} else if theLine.Command == "VERSION" {
		versionMsg := "NOTICE " + theLine.User + " :\u0001VERSION " + VERSION + "\u0001\n"
		self.SendRaw(versionMsg)
	}

	self.fromServer <- theLine
}

func (self *ircBot) IsRunning() bool {
	return self.isRunning
}

func (self *ircBot) Close() error {
	self.sendShutdown()
	self.isRunning = false
	return self.socket.Close()
}

// Send a non-standard SHUTDOWN message to the plugins
// This allows them to know that this channel is offline
func (self *ircBot) sendShutdown() {

	shutLine := &line.Line{
		Command:   "SHUTDOWN",
		Received:  time.Now().UTC().Format(time.RFC3339Nano),
		ChatBotId: self.id,
		User:      self.nick,
		Raw:       "",
		Content:   ""}

	for _, channel := range self.channels {
		shutLine.Channel = channel
		self.fromServer <- shutLine
	}
}

/*
 * UTIL
 */

// Split a string into sorted array of strings:
// e.g. "#bob, #alice" becomes ["#alice", "#bob"]
func splitChannels(rooms string) []string {

	var channels []string = make([]string, 0)
	for _, s := range strings.Split(rooms, ",") {
		channels = append(channels, strings.TrimSpace(s))
	}
	sort.Strings(channels)
	return channels
}

// Takes a raw string from IRC server and parses it
func parseLine(data string) (*line.Line, error) {

	var prefix, command, trailing, user, host, raw string
	var args, parts []string
	var isCTCP bool

	data = sane(data)

	if len(data) <= 2 {
		return nil, line.ELSHORT
	}

	raw = data
	if data[0] == ':' { // Do we have a prefix?
		parts = strings.SplitN(data[1:], " ", 2)
		if len(parts) != 2 {
			return nil, line.ELMALFORMED
		}

		prefix = parts[0]
		data = parts[1]

		if strings.Contains(prefix, "!") {
			parts = strings.Split(prefix, "!")
			if len(parts) != 2 {
				return nil, line.ELMALFORMED
			}
			user = parts[0]
			host = parts[1]

		} else {
			host = prefix
		}
	}

	if strings.Index(data, " :") != -1 {
		parts = strings.SplitN(data, " :", 2)
		if len(parts) != 2 {
			return nil, line.ELMALFORMED
		}
		data = parts[0]
		args = strings.Split(data, " ")

		trailing = parts[1]

		// IRC CTCP uses ascii null byte
		if len(trailing) > 0 && trailing[0] == '\001' {
			isCTCP = true
		}
		trailing = sane(trailing)

	} else {
		args = strings.Split(data, " ")
	}

	command = args[0]
	args = args[1:len(args)]

	channel := ""
	for _, arg := range args {
		if strings.HasPrefix(arg, "#") {
			channel = arg
			break
		}
	}

	if len(channel) == 0 {
		if command == "PRIVMSG" {
			// A /query or /msg message, channel is first arg
			channel = args[0]
		} else if command == "JOIN" {
			// JOIN commands say which channel in content part of msg
			channel = trailing
		}
	}

	if strings.HasPrefix(trailing, "ACTION") {
		// Received a /me line
		parts = strings.SplitN(trailing, " ", 2)
		if len(parts) != 2 {
			return nil, line.ELMALFORMED
		}
		trailing = parts[1]
		command = "ACTION"
	} else if strings.HasPrefix(trailing, "VERSION") {
		trailing = ""
		command = "VERSION"
	}

	theLine := &line.Line{
		ChatBotId: -1, // Set later
		Raw:       raw,
		Received:  time.Now().UTC().Format(time.RFC3339Nano),
		User:      user,
		Host:      host,
		Command:   command,
		Args:      args,
		Content:   trailing,
		IsCTCP:    isCTCP,
		Channel:   channel,
	}

	return theLine, nil
}

/* Trims a string to not include junk such as:
- the null bytes after a character return
- \n and \r
- whitespace
- Ascii char \001, which is the extended data delimiter,
  used for example in a /me command before 'ACTION'.
  See http://www.irchelp.org/irchelp/rfc/ctcpspec.html
- Null bytes: \000
*/
func sane(data string) string {
	parts := strings.SplitN(data, "\n", 2)
	return strings.Trim(parts[0], " \n\r\001\000")
}

// Converts an array of bytes to a string
// If the bytes are valid UTF-8, return those (as string),
// otherwise assume we have ISO-8859-1 (latin1, and kinda windows-1252),
// and use the bytes as unicode code points, because ISO-8859-1 is a
// subset of unicode
func toUnicode(data []byte) string {

	var result string

	if utf8.Valid(data) {
		result = string(data)
	} else {
		runes := make([]rune, len(data))
		for index, val := range data {
			runes[index] = rune(val)
		}
		result = string(runes)
	}

	return result
}

// Are a and b equal?
func isEqual(a, b []string) bool {
	ja := strings.Join(a, ",")
	jb := strings.Join(b, ",")
	return bytes.Equal([]byte(ja), []byte(jb))
}

// Is a in b? container must be sorted
func isIn(a string, container []string) bool {
	index := sort.SearchStrings(container, a)
	return index < len(container) && container[index] == a
}
