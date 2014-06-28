package irc

import (
	"flag"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/BotBotMe/botbot-bot/common"
	"github.com/BotBotMe/botbot-bot/line"
	"github.com/golang/glog"
)

var (
	NEW_CHANNEL = common.Channel{Name: "#unitnew", Fingerprint: "new-channel-uuid"}
)

// setGlogFlags walk around a glog issue and force it to log to stderr.
// It need to be called at the beginning of each test.
func setGlogFlags() {
	flag.Set("alsologtostderr", "true")
	flag.Set("V", "3")
}

func TestParseLine_welcome(t *testing.T) {
	setGlogFlags()

	line1 := ":barjavel.freenode.net 001 graham_king :Welcome to the freenode Internet Relay Chat Network graham_king"
	line, err := parseLine(line1)

	if err != nil {
		t.Error("parseLine error: ", err)
	}

	if line.Command != "001" {
		t.Error("Command incorrect")
	}
	if line.Host != "barjavel.freenode.net" {
		t.Error("Host incorrect")
	}
}

func TestParseLine_privmsg(t *testing.T) {
	setGlogFlags()
	line1 := ":rnowak!~rnowak@q.ovron.com PRIVMSG #linode :totally"
	line, err := parseLine(line1)

	if err != nil {
		t.Error("parseLine error: ", err)
	}

	if line.Command != "PRIVMSG" {
		t.Error("Command incorrect. Got", line.Command)
	}
	if line.Host != "~rnowak@q.ovron.com" {
		t.Error("Host incorrect. Got", line.Host)
	}
	if line.User != "rnowak" {
		t.Error("User incorrect. Got", line.User)
	}
	if line.Channel != "#linode" {
		t.Error("Channel incorrect. Got", line.Channel)
	}
	if line.Content != "totally" {
		t.Error("Content incorrect. Got", line.Content)
	}
}

func TestParseLine_pm(t *testing.T) {
	setGlogFlags()

	line1 := ":graham_king!graham_kin@i.love.debian.org PRIVMSG botbotme :hello"
	line, err := parseLine(line1)

	if err != nil {
		t.Error("parseLine error: ", err)
	}

	if line.Command != "PRIVMSG" {
		t.Error("Command incorrect. Got", line.Command)
	}
	if line.Channel != "botbotme" {
		t.Error("Channel incorrect. Got", line.Channel)
	}
	if line.Content != "hello" {
		t.Error("Content incorrect. Got", line.Content)
	}
}

func TestParseLine_list(t *testing.T) {
	setGlogFlags()
	line1 := ":oxygen.oftc.net 322 graham_king #linode 412 :Linode Community Support | http://www.linode.com/ | Linodes in Asia-Pacific! - http://bit.ly/ooBzhV"
	line, err := parseLine(line1)

	if err != nil {
		t.Error("parseLine error: ", err)
	}

	if line.Command != "322" {
		t.Error("Command incorrect. Got", line.Command)
	}
	if line.Host != "oxygen.oftc.net" {
		t.Error("Host incorrect. Got", line.Host)
	}
	if line.Channel != "#linode" {
		t.Error("Channel incorrect. Got", line.Channel)
	}
	if !strings.Contains(line.Content, "Community Support") {
		t.Error("Content incorrect. Got", line.Content)
	}
	if line.Args[2] != "412" {
		t.Error("Args incorrect. Got", line.Args)
	}
}

func TestParseLine_quit(t *testing.T) {
	setGlogFlags()
	line1 := ":nicolaslara!~nicolasla@c83-250-0-151.bredband.comhem.se QUIT :"
	line, err := parseLine(line1)
	if err != nil {
		t.Error("parse line error:", err)
	}
	if line.Command != "QUIT" {
		t.Error("Command incorrect. Got", line)
	}
}

func TestParseLine_part(t *testing.T) {
	setGlogFlags()
	line1 := ":nicolaslara!~nicolasla@c83-250-0-151.bredband.comhem.se PART #lincolnloop-internal"
	line, err := parseLine(line1)
	if err != nil {
		t.Error("parse line error:", err)
	}
	if line.Command != "PART" {
		t.Error("Command incorrect. Got", line)
	}
	if line.Channel != "#lincolnloop-internal" {
		t.Error("Channel incorrect. Got", line.Channel)
	}
}

func TestParseLine_353(t *testing.T) {
	setGlogFlags()
	line1 := ":hybrid7.debian.local 353 botbot = #test :@botbot graham_king"
	line, err := parseLine(line1)
	if err != nil {
		t.Error("parse line error:", err)
	}
	if line.Command != "353" {
		t.Error("Command incorrect. Got", line)
	}
	if line.Channel != "#test" {
		t.Error("Channel incorrect. Got", line.Channel)
	}
	if line.Content != "@botbot graham_king" {
		t.Error("Content incorrect. Got", line.Content)
	}
}

// Test sending messages too fast
func TestFlood(t *testing.T) {
	setGlogFlags()

	NUM := 5

	fromServer := make(chan *line.Line)
	receivedCounter := make(chan bool)
	mockSocket := common.MockSocket{Counter: receivedCounter}
	channels := make([]*common.Channel, 1)
	channels = append(channels, &common.Channel{Name: "test", Fingerprint: "uuid-string"})

	chatbot := &ircBot{
		id:               99,
		address:          "fakehost",
		nick:             "test",
		realname:         "Unit Test",
		password:         "test",
		serverIdentifier: "localhost.test",
		rateLimit:        time.Second,
		fromServer:       fromServer,
		channels:         channels,
		socket:           mockSocket,
		monitorChan:      make(chan struct{}),
		pingResponse:     make(chan struct{}, 10), // HACK: This is to avoid the current deadlock
		receive:          make(chan string),
		sendQueue:        make(chan []byte, 256),
	}
	chatbot.Init()

	startTime := time.Now()

	// Send the messages
	for i := 0; i < NUM; i++ {
		chatbot.Send("test", "Msg "+strconv.Itoa(i))
	}

	// Wait for them to 'arrive' at the socket
	for numGot := 0; numGot <= NUM; numGot++ {
		<-receivedCounter
	}

	elapsed := int64(time.Since(startTime))

	expected := int64((NUM-1)/4) * int64(chatbot.rateLimit)
	if elapsed < expected {
		t.Error("Flood prevention did not work")
	}

}

// Test joining additional channels
func TestUpdate(t *testing.T) {
	setGlogFlags()
	glog.Infoln("[DEBUG] starting TestUpdate")

	fromServer := make(chan *line.Line)
	receiver := make(chan string, 10)
	mockSocket := common.MockSocket{Receiver: receiver}
	channels := make([]*common.Channel, 0, 2)
	channel := common.Channel{Name: "#test", Fingerprint: "uuid-string"}
	channels = append(channels, &channel)

	chatbot := &ircBot{
		id:               99,
		address:          "localhost",
		nick:             "test",
		realname:         "Unit Test",
		password:         "test",
		serverIdentifier: "localhost.test1",
		fromServer:       fromServer,
		channels:         channels,
		socket:           mockSocket,
		rateLimit:        time.Second,
		monitorChan:      make(chan struct{}),
		pingResponse:     make(chan struct{}, 10), // HACK: This is to avoid the current deadlock
		receive:          make(chan string),
		sendQueue:        make(chan []byte, 256),
	}
	chatbot.Init()
	conf := map[string]string{
		"nick": "test", "password": "testxyz", "server": "localhost"}
	channels = append(channels, &NEW_CHANNEL)
	newConfig := &common.BotConfig{Id: 1, Config: conf, Channels: channels}

	// TODO (yml) there is probably better than sleeping but we need to wait
	// until chatbot is fully ready
	time.Sleep(time.Second * 2)
	chatbot.Update(newConfig)
	isFound := false
	for received := range mockSocket.Receiver {
		glog.Infoln("[DEBUG] received", received)
		if strings.TrimSpace(received) == "JOIN "+NEW_CHANNEL.Credential() {
			isFound = true
			close(mockSocket.Receiver)
		} else if received == "JOIN #test" {
			t.Error("Should not rejoin channels already in, can cause flood")
		}
	}
	if !isFound {
		t.Error("Expected JOIN " + NEW_CHANNEL.Credential())
	}
}

func TestToUnicodeUTF8(t *testing.T) {
	setGlogFlags()
	msg := "ελληνικά"
	result := toUnicode([]byte(msg))
	if result != msg {
		t.Error("UTF8 error.", msg, "became", result)
	}
}

func TestToUnicodeLatin1(t *testing.T) {
	setGlogFlags()
	msg := "âôé"
	latin1_bytes := []byte{0xe2, 0xf4, 0xe9}
	result := toUnicode(latin1_bytes)
	if result != msg {
		t.Error("ISO-8859-1 error.", msg, "became", result)
	}
}

func TestSplitChannels(t *testing.T) {
	setGlogFlags()
	input := "#aone, #btwo, #cthree"
	result := splitChannels(input)
	if len(result) != 3 || result[2] != "#cthree" {
		t.Error("Error. Splitting", input, "gave", result)
	}
}
