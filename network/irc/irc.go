// IRC server connection
//
// Connecting to an IRC server goes like this:
// 1. Connect to the socket. Wait for a response (anything will do).
// 2. Send USER and NICK. Wait for a response (anything).
// 2.5 If we have a password, wait for NickServ to ask for it, and to confirm authentication
// 3. JOIN channels

package irc

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"expvar"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/golang/glog"

	"github.com/BotBotMe/botbot-bot/common"
	"github.com/BotBotMe/botbot-bot/line"
)

const (
	VERSION = "botbotme v0.2"
)

type ircBot struct {
	sync.RWMutex
	id               int
	address          string
	socket           io.ReadWriteCloser
	nick             string
	realname         string
	password         string
	serverPass       string
	fromServer       chan *line.Line
	channels         []*common.Channel
	isRunning        bool
	isConnecting     bool
	isAuthenticating bool
	sendQueue        chan []byte
	monitorChan      chan struct{}
	pingResponse     chan struct{}
	reconnectChan    chan struct{}
	stats            *expvar.Map
}

func NewBot(config *common.BotConfig, fromServer chan *line.Line) common.ChatBot {

	// realname is set to config["realname"] or config["nick"]
	realname := config.Config["realname"]
	if realname == "" {
		realname = config.Config["nick"]
	}

	chatbot := &ircBot{
		id:            config.Id,
		address:       config.Config["server"],
		nick:          config.Config["nick"],
		realname:      realname,
		password:      config.Config["password"],        // NickServ password
		serverPass:    config.Config["server_password"], // PASS password
		fromServer:    fromServer,
		channels:      config.Channels,
		monitorChan:   make(chan struct{}),
		pingResponse:  make(chan struct{}),
		reconnectChan: make(chan struct{}),
		isRunning:     true,
		stats:         expvar.NewMap(fmt.Sprintf("%s_%d", config.Config["nick"], config.Id)),
	}

	chatbot.stats.Add("channels", 0)
	chatbot.stats.Add("messages", 0)
	chatbot.stats.Add("ping", 0)
	chatbot.stats.Add("pong", 0)
	chatbot.stats.Add("missed_ping", 0)
	chatbot.stats.Add("restart", 0)
	chatbot.Init()

	return chatbot
}

func (bot *ircBot) GetUser() string {
	return bot.nick
}

func (bot *ircBot) String() string {
	return fmt.Sprintf("%s on %s", bot.nick, bot.address)
}

// Monitor that we are still connected to the IRC server
// should run in go-routine
// If no message is received during 60 actively ping ircBot.
// If ircBot does not reply try to reconnect
// If ircBot replied 15 (maxPongWithoutMessage) but we are still not getting
// messages there is probably something wrong so we are going to try to
// reconnect.
func (bot *ircBot) monitor() {
	// TODO maxPongWithoutMessage should probably be a field of ircBot
	maxPongWithoutMessage := 150
	pongCounter := 0
	for bot.IsRunning() {
		if pongCounter > maxPongWithoutMessage {
			glog.Infoln("More than maxPongWithoutMessage pong without message for", bot)
			glog.Infoln("IRC monitoring KO", bot)
			bot.reconnect()
		}
		select {
		case <-bot.monitorChan:
			pongCounter = 0
			if glog.V(2) {
				glog.Infoln("Message received from the server for", bot)
			}
		case <-time.After(time.Second * 60):
			glog.Infoln("Ping the ircBot server", pongCounter, bot)
			bot.ping()
			select {
			case <-bot.pingResponse:
				bot.stats.Add("pong", 1)
				pongCounter += 1
				if glog.V(1) {
					glog.Infoln("Pong from ircBot server", bot)
				}
			case <-time.After(time.Second * 10):
				bot.stats.Add("missed_ping", 1)
				glog.Infoln("No pong from ircBot server", bot)
				// Do not kill the server on the first missed PONG
				pongCounter += 15
			}
		}
	}
}

// reconnect the ircBot
func (bot *ircBot) reconnect() {
	glog.Infoln("Trying to reconnect", bot)
	bot.stats.Add("restart", 1)
	bot.Close()
	time.Sleep(1 * time.Second) // Wait for timeout to be sure listen has stopped
	bot.Init()

}

// Ping the server to  the connection open
func (bot *ircBot) ping() {
	// TODO increment number
	bot.stats.Add("ping", 1)
	bot.SendRaw("PING 1")
}

// Connect to the IRC server and start listener
func (bot *ircBot) Init() {

	bot.isConnecting = true
	bot.isAuthenticating = false

	bot.Connect()

	// Listen for incoming messages in background thread
	go bot.listen()

	// Monitor that we are still getting incoming messages in a background thread
	go bot.monitor()

	// Listen for outgoing messages (rate limited) in background thread
	if bot.sendQueue == nil {
		bot.sendQueue = make(chan []byte, 256)
	}
	go bot.sender()

	if bot.serverPass != "" {
		bot.SendRaw("PASS " + bot.serverPass)
	}

	bot.SendRaw("PING Bonjour")
}

// Connect to the server. Here we keep trying every 10 seconds until we manage
// to Dial to the server.
func (bot *ircBot) Connect() {

	if bot.socket != nil {
		// socket already set, unit tests need this
		return
	}

	var socket net.Conn
	var err error

	for {
		glog.Infoln("Connecting to IRC server: ", bot.address)

		socket, err = tls.Dial("tcp", bot.address, nil) // Always try TLS first
		if err == nil {
			glog.Infoln("Connected: TLS secure")
			break
		}

		glog.Infoln("Could not connect using TLS because: ", err)

		_, ok := err.(x509.HostnameError)
		if ok {
			// Certificate might not match. This happens on irc.cloudfront.net
			insecure := &tls.Config{InsecureSkipVerify: true}
			socket, err = tls.Dial("tcp", bot.address, insecure)

			if err == nil && isCertValid(socket.(*tls.Conn)) {
				glog.Infoln("Connected: TLS with awkward certificate")
				break
			}
		}

		socket, err = net.Dial("tcp", bot.address)

		if err == nil {
			glog.Infoln("Connected: Plain text insecure")
			break
		}

		glog.Infoln("IRC Connect error. Will attempt to re-connect. ", err)
		time.Sleep(10 * time.Second)
	}

	bot.socket = socket
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
	glog.Infoln("Checking IP of", hostname)

	addrs, err := net.LookupIP(hostname)
	if err != nil {
		glog.Errorln("Error DNS lookup of "+hostname+": ", err)
		return false
	}

	for _, ip := range addrs {
		if ip.String() == connIP {
			glog.Infoln("Accepting certificate anyway. " + hostname + " has same IP as connection")
			return true
		}
	}
	return false
}

// Update bot configuration. Called when webapp changes a chatbot's config.
func (bot *ircBot) Update(config *common.BotConfig) {

	isNewServer := bot.updateServer(config)
	if isNewServer {
		// If the server changed, we've already done nick and channel changes too
		return
	}

	bot.updateNick(config.Config["nick"], config.Config["password"])
	bot.updateChannels(config.Channels)
}

// Update the IRC server we're connected to
func (bot *ircBot) updateServer(config *common.BotConfig) bool {

	addr := config.Config["server"]
	if addr == bot.address {
		return false
	}

	glog.Infoln("Changing IRC server from ", bot.address, " to ", addr)

	bot.Close()
	time.Sleep(1 * time.Second) // Wait for timeout to be sure listen has stopped

	bot.address = addr
	bot.nick = config.Config["nick"]
	bot.password = config.Config["password"]
	bot.channels = config.Channels

	bot.Init()

	return true
}

// Update the nickname we're registered under, if needed
func (bot *ircBot) updateNick(newNick, newPass string) {
	if newNick == bot.nick {
		return
	}

	bot.nick = newNick
	bot.password = newPass
	bot.setNick()
}

// Update the channels based on new configuration, leaving old ones and joining new ones
func (bot *ircBot) updateChannels(newChannels []*common.Channel) {
	if isEqual(newChannels, bot.channels) {
		if glog.V(2) {
			glog.Infoln("Channels comparison is equals for bot: ", bot.nick)
		}
		return
	}

	// PART old ones
	for _, channel := range bot.channels {
		if !isIn(channel, newChannels) {
			bot.part(channel.Credential())
		}
	}

	// JOIN new ones
	for _, channel := range newChannels {
		if !isIn(channel, bot.channels) {
			bot.join(channel.Credential())
		}
	}

	bot.channels = newChannels
}

// Join channels
func (bot *ircBot) JoinAll() {
	for _, channel := range bot.channels {
		bot.join(channel.Credential())
	}
}

// Join an IRC channel
func (bot *ircBot) join(channel string) {
	bot.SendRaw("JOIN " + channel)
	bot.stats.Add("channels", 1)
}

// Leave an IRC channel
func (bot *ircBot) part(channel string) {
	bot.SendRaw("PART " + channel)
	bot.stats.Add("channels", -1)
}

// Send a regular (non-system command) IRC message
func (bot *ircBot) Send(channel, msg string) {
	fullmsg := "PRIVMSG " + channel + " :" + msg
	bot.SendRaw(fullmsg)
}

// Send message down socket. Add \n at end first.
func (bot *ircBot) SendRaw(msg string) {
	bot.sendQueue <- []byte(msg + "\n")
}

// Tell the irc server who we are - we can't do anything until this is done.
func (bot *ircBot) login() {

	bot.isAuthenticating = true

	// We use the botname as the 'realname', because bot's don't have real names!
	bot.SendRaw("USER " + bot.nick + " 0 * :" + bot.realname)

	bot.setNick()
}

// Tell the network our
func (bot *ircBot) setNick() {
	bot.SendRaw("NICK " + bot.nick)
}

// Tell NickServ our password
func (bot *ircBot) sendPassword() {
	bot.Send("NickServ", "identify "+bot.password)
}

// Actually really send message to the server. Implements rate limiting.
// Should run in go-routine.
func (bot *ircBot) sender() {

	var data []byte
	var tempo = time.Second * 1
	var err error

	for bot.IsRunning() {

		data = <-bot.sendQueue
		if glog.V(1) {
			glog.Infoln("[RAW"+strconv.Itoa(bot.id)+"] -->", string(data))
		}

		_, err = bot.socket.Write(data)
		if err != nil {
			glog.Errorln("Error writing to socket to", bot, ": ", err)
			bot.reconnect()
		}
		bot.stats.Add("messages", 1)

		// Rate limit to one message every tempo
		// https://github.com/BotBotMe/botbot-bot/issues/2
		time.Sleep(tempo)
	}
}

// Listen for incoming messages. Parse them and put on channel.
// Should run in go-routine
func (bot *ircBot) listen() {

	var contentData []byte
	var content string
	var err error

	bufRead := bufio.NewReader(bot.socket)
	for bot.IsRunning() {
		contentData, err = bufRead.ReadBytes('\n')

		if err != nil {
			netErr, ok := err.(net.Error)
			if ok && netErr.Timeout() == true {
				continue

			} else if !bot.IsRunning() {
				// Close() wants us to stop
				return

			} else {
				glog.Errorln("Lost IRC server connection. ", err)
				bot.Close()
				return
			}
		}

		if len(contentData) == 0 {
			continue
		}

		content = toUnicode(contentData)

		if glog.V(2) {
			glog.Infoln("[RAW" + strconv.Itoa(bot.id) + "]" + content)
		}

		theLine, err := parseLine(content)
		if err == nil {
			theLine.ChatBotId = bot.id
			bot.act(theLine)
		} else {
			glog.Errorln("Invalid line:", content)
		}

	}
}

func (bot *ircBot) act(theLine *line.Line) {
	// Notify the monitor goroutine that we receive a PONG
	if theLine.Command == "PONG" {
		if glog.V(2) {
			glog.Infoln("Sending the signal in bot.pingResponse")
		}
		bot.pingResponse <- struct{}{}
		return
	}

	// Send the heartbeat to the monitoring goroutine via monitorChan
	bot.monitorChan <- struct{}{}

	// As soon as we receive a message from the server, complete initiatization
	if bot.isConnecting {
		bot.isConnecting = false
		bot.login()
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
			bot.sendPassword()
			return

		} else if isConfirm {
			bot.isAuthenticating = false
			bot.JoinAll()
			return
		}
	}

	// After USER / NICK is accepted, join all the channels,
	// assuming we don't need to identify with NickServ

	if bot.isAuthenticating && len(bot.password) == 0 {
		bot.isAuthenticating = false
		bot.JoinAll()
		return
	}

	if theLine.Command == "PING" {
		// Reply, and send message on to client
		bot.SendRaw("PONG " + theLine.Content)
	} else if theLine.Command == "VERSION" {
		versionMsg := "NOTICE " + theLine.User + " :\u0001VERSION " + VERSION + "\u0001\n"
		bot.SendRaw(versionMsg)
	}

	bot.fromServer <- theLine
}

func (bot *ircBot) IsRunning() bool {
	bot.RLock()
	defer bot.RUnlock()
	return bot.isRunning
}

// Close ircBot
func (bot *ircBot) Close() error {
	bot.Lock()
	defer bot.Unlock()
	bot.isRunning = false
	bot.sendShutdown()
	return bot.socket.Close()
}

// Send a non-standard SHUTDOWN message to the plugins
// This allows them to know that this channel is offline
func (bot *ircBot) sendShutdown() {
	shutLine := &line.Line{
		Command:   "SHUTDOWN",
		Received:  time.Now().UTC().Format(time.RFC3339Nano),
		ChatBotId: bot.id,
		User:      bot.nick,
		Raw:       "",
		Content:   ""}

	for _, channel := range bot.channels {
		shutLine.Channel = channel.Credential()
		bot.fromServer <- shutLine
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
func isEqual(a, b []*common.Channel) (flag bool) {
	for _, a_cc := range a {
		flag = false
		for _, b_cc := range b {
			if a_cc.Fingerprint == b_cc.Fingerprint {
				flag = true
				break
			}
		}
		if flag == false {
			return flag
		}
	}
	return true
}

// Is a in b? container must be sorted
func isIn(a *common.Channel, container []*common.Channel) (flag bool) {
	flag = false
	for _, c_cc := range container {
		if a.Fingerprint == c_cc.Fingerprint {
			flag = true
			break
		}
	}
	if flag == false {
		return flag
	}
	return true
}
