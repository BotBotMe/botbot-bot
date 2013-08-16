package irc

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"botbot-bot/common"
	"botbot-bot/line"
)

const (
	NEW_CHANNEL = "#unitnew"
)

func TestParseLine_welcome(t *testing.T) {

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

// Dummy implementation of ReadWriteCloser
type MockSocket struct {
	received []string
	counter  chan bool
}

func (self *MockSocket) Write(data []byte) (int, error) {
	self.received = append(self.received, string(data))
	if self.counter != nil {
		self.counter <- true
	}
	return len(data), nil
}

func (self *MockSocket) Read(into []byte) (int, error) {
	time.Sleep(time.Second) // Prevent busy loop
	return 0, nil
}

func (self *MockSocket) Close() error {
	return nil
}

// Test sending messages too fast
func TestFlood(t *testing.T) {

	NUM := 5

	fromServer := make(chan *line.Line)
	receivedCounter := make(chan bool)
	mockSocket := MockSocket{counter: receivedCounter}

	chatbot := &ircBot{
		id:         99,
		address:    "localhost",
		nick:       "test",
		realname:   "Unit Test",
		password:   "test",
		fromServer: fromServer,
		channels:   []string{"test"},
		socket:     &mockSocket,
		isRunning:  true,
	}
	chatbot.init()

	startTime := time.Now()

	// Send the messages
	for i := 0; i < NUM; i++ {
		chatbot.Send("test", "Msg "+strconv.Itoa(i))
	}

	// Wait for them to 'arrive' at the socket
	for numGot := 0; numGot < NUM; numGot++ {
		<-receivedCounter
	}

	elapsed := int64(time.Since(startTime))

	// NUM messages should take at least ((NUM-1) / 4) seconds (max 4 msgs second)
	expected := int64((NUM-1)/4) * int64(time.Second)
	if elapsed < expected {
		t.Error("Flood prevention did not work")
	}

}

// Test joining additional channels
func TestUpdate(t *testing.T) {

	fromServer := make(chan *line.Line)
	mockSocket := MockSocket{counter: nil}

	chatbot := &ircBot{
		id:         99,
		address:    "localhost",
		nick:       "test",
		realname:   "Unit Test",
		password:   "test",
		fromServer: fromServer,
		channels:   []string{"#test"},
		socket:     &mockSocket,
		sendQueue:  make(chan []byte, 100),
		isRunning:  true,
	}
	// Rate limiting requires a go-routine to actually do the sending
	go chatbot.sender()

	conf := map[string]string{
		"nick": "test", "password": "testxyz", "server": "localhost"}
	channels := []string{"#test", NEW_CHANNEL}
	newConfig := &common.BotConfig{Id: 1, Config: conf, Channels: channels}

	chatbot.Update(newConfig)

	// Wait a bit
	for i := 0; i < 10; i++ {
		time.Sleep(time.Second / 3)
		if len(mockSocket.received) >= 1 {
			break
		}
	}

	// Expect a JOIN of NEW_CHANNEL but NOT a JOIN on #test (because already in there)
	isFound := false
	for _, cmd := range mockSocket.received {

		cmd = strings.TrimSpace(cmd)

		if cmd == "JOIN "+NEW_CHANNEL {
			isFound = true
			break
		}

		if cmd == "JOIN #test" {
			t.Error("Should not rejoin channels already in, can cause flood")
		}
	}

	if !isFound {
		t.Error("Expected JOIN " + NEW_CHANNEL)
	}
}

func TestToUnicodeUTF8(t *testing.T) {
	msg := "ελληνικά"
	result := toUnicode([]byte(msg))
	if result != msg {
		t.Error("UTF8 error.", msg, "became", result)
	}
}

func TestToUnicodeLatin1(t *testing.T) {
	msg := "âôé"
	latin1_bytes := []byte{0xe2, 0xf4, 0xe9}
	result := toUnicode(latin1_bytes)
	if result != msg {
		t.Error("ISO-8859-1 error.", msg, "became", result)
	}
}

func TestSplitChannels(t *testing.T) {
	input := "#aone, #btwo, #cthree"
	result := splitChannels(input)
	if len(result) != 3 || result[2] != "#cthree" {
		t.Error("Error. Splitting", input, "gave", result)
	}
}
