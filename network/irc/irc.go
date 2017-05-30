// IRC server connection
//
// Connecting to an IRC server goes like this:
// 1. Connect to the conn. Wait for a response (anything will do).
// 2. Send USER and NICK. Wait for a response (anything).
// 2.5 If we have a password, wait for NickServ to ask for it, and to confirm authentication
// 3. JOIN channels

package irc

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"expvar"
	"fmt"
	"io"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/golang/glog"

	"github.com/BotBotMe/botbot-bot/common"
	"github.com/BotBotMe/botbot-bot/line"
)

const (
	// VERSION of the botbot-bot
	VERSION = "botbot v0.3.0"
	// RPL_WHOISCHANNELS IRC command code from the spec
	RPL_WHOISCHANNELS = "319"
)

type chatBotStats struct {
	sync.RWMutex
	m map[string]*expvar.Map
}

func (s *chatBotStats) GetOrCreate(identifier string) (*expvar.Map, bool) {
	s.RLock()
	chatbotStats, ok := s.m[identifier]
	s.RUnlock()
	if !ok {
		chatbotStats = expvar.NewMap(identifier)
		s.Lock()
		s.m[identifier] = chatbotStats
		s.Unlock()
	}
	return chatbotStats, ok
}

var (
	// BotStats hold the references to the expvar.Map for each ircBot instance
	BotStats = chatBotStats{m: make(map[string]*expvar.Map)}
)

type ircBot struct {
	sync.RWMutex
	id               int
	address          string
	nick             string
	realname         string
	password         string
	serverPass       string
	serverIdentifier string
	rateLimit        time.Duration // Duration used to rate limit send
	channels         []*common.Channel
	isConnecting     bool
	isAuthenticating bool
	isClosed         bool
	sendQueue        chan []byte
	fromServer       chan *line.Line
	pingResponse     chan struct{}
	closing          chan struct{}
}

// NewBot create an irc instance of ChatBot
func NewBot(config *common.BotConfig, fromServer chan *line.Line) common.ChatBot {
	// realname is set to config["realname"] or config["nick"]
	realname := config.Config["realname"]
	if realname == "" {
		realname = config.Config["nick"]
	}

	// Initialize the bot.
	chatbot := &ircBot{
		id:               config.Id,
		address:          config.Config["server"],
		nick:             config.Config["nick"],
		realname:         realname,
		password:         config.Config["password"],        // NickServ password
		serverPass:       config.Config["server_password"], // PASS password
		serverIdentifier: config.Config["server_identifier"],
		rateLimit:        time.Second,
		fromServer:       fromServer,
		channels:         config.Channels,
		pingResponse:     make(chan struct{}, 10), // HACK: This is to avoid the current deadlock
		sendQueue:        make(chan []byte, 256),
		closing:          make(chan struct{}),
	}

	chatbotStats, ok := BotStats.GetOrCreate(chatbot.serverIdentifier)

	// Initialize the counter for the exported variable
	if !ok {
		chatbotStats.Add("channels", 0)
		chatbotStats.Add("sent_messages", 0)
		chatbotStats.Add("received_messages", 0)
		chatbotStats.Add("ping", 0)
		chatbotStats.Add("pong", 0)
		chatbotStats.Add("missed_ping", 0)
		chatbotStats.Add("restart", 0)
		chatbotStats.Add("reply_whoischannels", 0)
	}

	conn := chatbot.connect()
	chatbot.init(conn)
	return chatbot
}

// GetUser returns the bot.nick
func (bot *ircBot) GetUser() string {
	bot.RLock()
	defer bot.RUnlock()
	return bot.nick
}

// IsRunning the isRunning field
func (bot *ircBot) IsRunning() bool {
	bot.RLock()
	defer bot.RUnlock()
	return !bot.isClosed
}

// GetStats returns the expvar.Map for this bot
func (bot *ircBot) GetStats() *expvar.Map {
	stats, _ := BotStats.GetOrCreate(bot.serverIdentifier)
	return stats
}

// String returns the string representation of the bot
func (bot *ircBot) String() string {
	bot.RLock()
	defer bot.RUnlock()
	return fmt.Sprintf("%s on %s (%p)", bot.nick, bot.address, bot)
}

// listenSendMonitor is the main goroutine of the ircBot it listens to the conn
// send response to irc via the conn and it check that the conn is healthy if
// it is not it try to reconnect.
func (bot *ircBot) listenSendMonitor(quit chan struct{}, receive chan string, conn io.ReadWriteCloser) {
	var pingTimeout <-chan time.Time
	reconnect := make(chan struct{})
	// TODO maxPongWithoutMessage should probably be a field of ircBot
	maxPingWithoutResponse := 1 // put it back to 3
	maxPongWithoutMessage := 150
	pongCounter := 0
	missedPing := 0
	whoisTimerChan := time.After(time.Minute * 5)

	botStats := bot.GetStats()
	for {
		select {
		case <-quit:
			return
		case <-reconnect:
			glog.Infoln("IRC monitoring KO shutting down", bot)
			botStats.Add("restart", 1)
			err := bot.Close()
			if err != nil {
				glog.Errorln("An error occured while Closing the bot", bot, ": ", err)
			}
			return
		case <-whoisTimerChan:
			bot.Whois()
			whoisTimerChan = time.After(time.Minute * 5)
		case <-time.After(time.Second * 60):
			glog.Infoln("[Info] Ping the ircBot server", pongCounter, bot)
			botStats.Add("ping", 1)
			bot.SendRaw("PING 1")
			// Activate the ping timeout case
			pingTimeout = time.After(time.Second * 10)
		case <-bot.pingResponse:
			// deactivate the case waiting for a pingTimeout because we got a response
			pingTimeout = nil
			botStats.Add("pong", 1)
			pongCounter++
			if glog.V(1) {
				glog.Infoln("[Info] Pong from ircBot server", bot)
			}
			if pongCounter > maxPongWithoutMessage {
				close(reconnect)
			}
		case <-pingTimeout:
			// Deactivate the pingTimeout case
			pingTimeout = nil
			botStats.Add("missed_ping", 1)
			missedPing++
			glog.Infoln("[Info] No pong from ircBot server", bot, "missed", missedPing)
			if missedPing > maxPingWithoutResponse {
				close(reconnect)
			}
		case content := <-receive:
			theLine, err := parseLine(content)
			if err == nil {
				theLine.BotNick = bot.nick
				botStats.Add("received_messages", 1)
				bot.RLock()
				theLine.ChatBotId = bot.id
				bot.RUnlock()
				bot.act(theLine)
				pongCounter = 0
				missedPing = 0
				// Deactivate the pingTimeout case
				pingTimeout = nil

			} else {
				glog.Errorln("Invalid line:", content)
			}
		// Rate limit to one message every tempo
		// // https://github.com/BotBotMe/botbot-bot/issues/2
		case data := <-bot.sendQueue:
			glog.V(3).Infoln(bot, " Pulled data from bot.sendQueue chan:", string(data))
			if glog.V(2) {
				glog.Infoln("[RAW", bot, "] -->", string(data))
			}
			_, err := conn.Write(data)
			if err != nil {
				glog.Errorln("Error writing to conn to", bot, ": ", err)
				close(reconnect)
			}
			botStats.Add("sent_messages", 1)
			time.Sleep(bot.rateLimit)

		}
	}
}

// init initializes the conn to the ircServer and start all the gouroutines
// requires to run ircBot
func (bot *ircBot) init(conn io.ReadWriteCloser) {
	glog.Infoln("Init bot", bot)

	quit := make(chan struct{})
	receive := make(chan string)

	go bot.readSocket(quit, receive, conn)

	// Listen for incoming messages in background thread
	go bot.listenSendMonitor(quit, receive, conn)

	go func(bot *ircBot, conn io.Closer) {
		for {
			select {
			case <-bot.closing:
				err := conn.Close()
				if err != nil {
					glog.Errorln("An error occured while closing the conn of", bot, err)
				}
				close(quit)
				return
			}
		}
	}(bot, conn)

	bot.RLock()
	if bot.serverPass != "" {
		bot.SendRaw("PASS " + bot.serverPass)
	}
	bot.RUnlock()

	bot.SendRaw("PING Bonjour")
}

// connect to the server. Here we keep trying every 10 seconds until we manage
// to Dial to the server.
func (bot *ircBot) connect() (conn io.ReadWriteCloser) {

	var (
		err     error
		counter int
	)

	connectTimeout := time.After(0)

	bot.Lock()
	bot.isConnecting = true
	bot.isAuthenticating = false
	bot.Unlock()

	for {
		select {
		case <-connectTimeout:
			counter++
			connectTimeout = nil
			glog.Infoln("[Info] Connecting to IRC server: ", bot.address)
			conn, err = tls.Dial("tcp", bot.address, nil) // Always try TLS first
			if err == nil {
				glog.Infoln("Connected: TLS secure")
				return conn
			} else if _, ok := err.(x509.HostnameError); ok {
				glog.Errorln("Could not connect using TLS because: ", err)
				// Certificate might not match. This happens on irc.cloudfront.net
				insecure := &tls.Config{InsecureSkipVerify: true}
				conn, err = tls.Dial("tcp", bot.address, insecure)

				if err == nil && isCertValid(conn.(*tls.Conn)) {
					glog.Errorln("Connected: TLS with awkward certificate")
					return conn
				}
			} else if _, ok := err.(x509.UnknownAuthorityError); ok {
				glog.Errorln("x509.UnknownAuthorityError : ", err)
				insecure := &tls.Config{InsecureSkipVerify: true}
				conn, err = tls.Dial("tcp", bot.address, insecure)
				if err == nil {
					glog.Infoln("Connected: TLS with an x509.UnknownAuthorityError", err)
					return conn
				}
			} else {
				glog.Errorln("Could not establish a tls connection", err)

			}

			conn, err = net.Dial("tcp", bot.address)
			if err == nil {
				glog.Infoln("Connected: Plain text insecure")
				return conn
			}
			// TODO (yml) At some point we might want to panic
			delay := 5 * counter
			glog.Infoln("IRC Connect error. Will attempt to re-connect. ", err, "in", delay, "seconds")
			connectTimeout = time.After(time.Duration(delay) * time.Second)
		}
	}
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

	}
	// Cert has several valid names
	for _, certname := range cert.DNSNames {
		if isIPMatch(certname, connAddr) {
			return true
		}
	}

	return false
}

// Does hostname have IP address connIP?
func isIPMatch(hostname string, connIP string) bool {
	glog.Infoln("Checking IP of", hostname)

	addrs, err := net.LookupIP(hostname)
	if err != nil {
		glog.Errorln("Error DNS lookup of ", hostname, ": ", err)
		return false
	}

	for _, ip := range addrs {
		if ip.String() == connIP {
			glog.Infoln("Accepting certificate anyway. ", hostname, " has same IP as connection")
			return true
		}
	}
	return false
}

// Update bot configuration. Called when webapp changes a chatbot's config.
func (bot *ircBot) Update(config *common.BotConfig) {

	isNewServer := bot.updateServer(config)
	if isNewServer {
		glog.Infoln("[Info] the config is from a new server.")
		// If the server changed, we've already done nick and channel changes too
		return
	}
	glog.Infoln("[Info] bot.Update -- It is not a new server.")

	bot.updateNick(config.Config["nick"], config.Config["password"])
	bot.updateChannels(config.Channels)
}

// Update the IRC server we're connected to
func (bot *ircBot) updateServer(config *common.BotConfig) bool {

	addr := config.Config["server"]
	if addr == bot.address {
		return false
	}

	glog.Infoln("[Info] Changing IRC server from ", bot.address, " to ", addr)

	err := bot.Close()
	if err != nil {
		glog.Errorln("An error occured while Closing the bot", bot, ": ", err)
	}

	bot.address = addr
	bot.nick = config.Config["nick"]
	bot.password = config.Config["password"]
	bot.channels = config.Channels

	conn := bot.connect()
	bot.init(conn)

	return true
}

// Update the nickname we're registered under, if needed
func (bot *ircBot) updateNick(newNick, newPass string) {
	glog.Infoln("[Info] Starting bot.updateNick()")

	bot.RLock()
	nick := bot.nick
	bot.RUnlock()
	if newNick == nick {
		glog.Infoln("[Info] bot.updateNick() -- the nick has not changed so return")
		return
	}
	glog.Infoln("[Info] bot.updateNick() -- set the new nick")

	bot.Lock()
	bot.nick = newNick
	bot.password = newPass
	bot.Unlock()
	bot.setNick()
}

// Update the channels based on new configuration, leaving old ones and joining new ones
func (bot *ircBot) updateChannels(newChannels []*common.Channel) {
	glog.Infoln("[Info] Starting bot.updateChannels")
	bot.RLock()
	channels := bot.channels
	bot.RUnlock()

	glog.V(3).Infoln("[Debug] newChannels: ", newChannels, "bot.channels:", channels)

	if isEqual(newChannels, channels) {
		if glog.V(2) {
			glog.Infoln("Channels comparison is equals for bot: ", bot.nick)
		}
		return
	}
	glog.Infoln("[Info] The channels the bot is connected to need to be updated")

	// PART old ones
	for _, channel := range channels {
		if !isIn(channel, newChannels) {
			glog.Infoln("[Info] Parting new channel: ", channel.Credential())
			bot.part(channel.Credential())
		}
	}

	// JOIN new ones
	for _, channel := range newChannels {
		if !isIn(channel, channels) {
			glog.Infoln("[Info] Joining new channel: ", channel.Credential())
			bot.join(channel.Credential())
		}
	}
	bot.Lock()
	bot.channels = newChannels
	bot.Unlock()
}

// Join channels
func (bot *ircBot) JoinAll() {
	for _, channel := range bot.channels {
		bot.join(channel.Credential())
	}
}

// Whois is used to query information about the bot
func (bot *ircBot) Whois() {
	// reset channel count so we can add them up from the response
	botStats := bot.GetStats()
	botStats.Set("reply_whoischannels", new(expvar.Int))
	bot.SendRaw("WHOIS " + bot.nick)
}

// Join an IRC channel
func (bot *ircBot) join(channel string) {
	bot.SendRaw("JOIN " + channel)
	botStats := bot.GetStats()
	botStats.Add("channels", 1)
}

// Leave an IRC channel
func (bot *ircBot) part(channel string) {
	bot.SendRaw("PART " + channel)
	botStats := bot.GetStats()
	botStats.Add("channels", -1)
}

// Send a regular (non-system command) IRC message
func (bot *ircBot) Send(channel, msg string) {
	fullmsg := "PRIVMSG " + channel + " :" + msg
	bot.SendRaw(fullmsg)
}

// Send message down conn. Add \n at end first.
func (bot *ircBot) SendRaw(msg string) {
	bot.sendQueue <- []byte(msg + "\n")
}

// Tell the irc server who we are - we can't do anything until this is done.
func (bot *ircBot) login() {

	bot.isAuthenticating = true

	bot.SendRaw("CAP REQ :sasl")
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

func (bot *ircBot) sendSaslStart() {
	bot.SendRaw("AUTHENTICATE PLAIN")

}

func (bot *ircBot) sendSaslPass() {
	out := bytes.Join([][]byte{[]byte(bot.nick), []byte(bot.nick), []byte(bot.password)}, []byte{0})
	encpass := base64.StdEncoding.EncodeToString(out)
	bot.SendRaw("AUTHENTICATE " + encpass)
}

func (bot *ircBot) sendSaslEnd() {
	bot.SendRaw("CAP END")
}

// Read from the conn
func (bot *ircBot) readSocket(quit chan struct{}, receive chan string, conn io.ReadWriteCloser) {

	bufRead := bufio.NewReader(conn)
	for {
		select {
		case <-quit:
			return
		default:
			contentData, err := bufRead.ReadBytes('\n')
			if err != nil {
				netErr, ok := err.(net.Error)
				if ok && netErr.Timeout() == true {
					continue
				} else {
					glog.Errorln("An Error occured while reading from conn ", err)
					return
				}
			}

			if len(contentData) == 0 {
				continue
			}

			content := toUnicode(contentData)
			if glog.V(2) {
				glog.Infoln("[RAW", bot, "] <--", content)
			}
			receive <- content
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

	bot.RLock()
	isConnecting := bot.isConnecting
	bot.RUnlock()
	// As soon as we receive a message from the server, complete initiatization
	if isConnecting {
		bot.Lock()
		bot.isConnecting = false
		bot.Unlock()
		bot.login()
		return
	}

	isAskingForSasl := theLine.User == "" && strings.HasSuffix(theLine.Raw, " CAP * ACK :sasl")
	if isAskingForSasl {
		bot.sendSaslStart()
		return
	}

	isAskingForSaslPass := theLine.User == "" && theLine.Raw == "AUTHENTICATE +"
	if isAskingForSaslPass {
		bot.sendSaslPass()
		return
	}

	isSaslConfirm := theLine.User == "" && theLine.Content == "SASL authentication successful"
	// After SASL is accepted, join all the channels
	if isSaslConfirm {
		bot.sendSaslEnd()
		bot.Lock()
		bot.isAuthenticating = false
		bot.Unlock()
		bot.JoinAll()
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
			bot.Lock()
			bot.isAuthenticating = false
			bot.Unlock()
			bot.JoinAll()
			return
		}
	}

	// After USER / NICK is accepted, join all the channels,
	// assuming we don't need to identify with NickServ
	bot.RLock()
	shouldIdentify := bot.isAuthenticating && len(bot.password) == 0
	bot.RUnlock()
	if shouldIdentify {
		bot.Lock()
		bot.isAuthenticating = false
		bot.Unlock()
		bot.JoinAll()
		return
	}

	if theLine.Command == "PING" {
		// Reply, and send message on to client
		bot.SendRaw("PONG " + theLine.Content)
	} else if theLine.Command == "VERSION" {
		versionMsg := "NOTICE " + theLine.User + " :\u0001VERSION " + VERSION + "\u0001\n"
		bot.SendRaw(versionMsg)
	} else if theLine.Command == RPL_WHOISCHANNELS {
		glog.Infoln("[Info] reply_whoischannels -- len:",
			len(strings.Split(theLine.Content, " ")), "content:", theLine.Content)
		botStats := bot.GetStats()
		botStats.Add("reply_whoischannels", int64(len(strings.Split(theLine.Content, " "))))
	}

	bot.fromServer <- theLine
}

// Close ircBot
func (bot *ircBot) Close() (err error) {
	// Send a signal to all goroutine to return
	glog.Infoln("[Info] Closing bot.")
	bot.sendShutdown()
	close(bot.closing)
	bot.Lock()
	bot.isClosed = true
	bot.Unlock()
	botStats := bot.GetStats()
	// zero out gauges
	botStats.Set("channels", new(expvar.Int))
	botStats.Set("reply_whoischannels", new(expvar.Int))
	return err
}

// Send a non-standard SHUTDOWN message to the plugins
// This allows them to know that this channel is offline
func (bot *ircBot) sendShutdown() {
	glog.Infoln("[Info] Logging Shutdown command in the channels monitored by:", bot)
	bot.RLock()
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
	bot.RUnlock()
}

/*
 * UTIL
 */

// Split a string into sorted array of strings:
// e.g. "#bob, #alice" becomes ["#alice", "#bob"]
func splitChannels(rooms string) []string {
	var channels = make([]string, 0)
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
		return nil, line.ErrLineShort
	}

	raw = data
	if data[0] == ':' { // Do we have a prefix?
		parts = strings.SplitN(data[1:], " ", 2)
		if len(parts) != 2 {
			return nil, line.ErrLineMalformed
		}

		prefix = parts[0]
		data = parts[1]

		if strings.Contains(prefix, "!") {
			parts = strings.Split(prefix, "!")
			if len(parts) != 2 {
				return nil, line.ErrLineMalformed
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
			return nil, line.ErrLineMalformed
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
			return nil, line.ErrLineMalformed
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
	if len(a) == len(b) {
		for _, aCc := range a {
			flag = false
			for _, bCc := range b {
				if aCc.Fingerprint == bCc.Fingerprint {
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
	return false
}

// Is a in b? container must be sorted
func isIn(a *common.Channel, channels []*common.Channel) (flag bool) {
	flag = false
	for _, cc := range channels {
		if a.Fingerprint == cc.Fingerprint {
			flag = true
			break
		}
	}
	if flag == false {
		return flag
	}
	return true
}
