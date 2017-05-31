package main

import (
	"strings"
	"testing"
	"time"

	"github.com/BotBotMe/botbot-bot/common"
	"github.com/golang/glog"
)

const (
	SERVER_PORT = "60667"
	TEST_MSG    = "q: Something new"
)

func GetQueueLength(queue *common.MockQueue) int {
	queue.RLock()
	q := queue.Got["q"]
	queue.RUnlock()
	return len(q)
}

// TODO (yml) this test  is broken because ircBot establish first a tls conn
// we need to find a better way to handle this.

// A serious integration test for BotBot.
// This covers BotBot, the IRC code, and the dispatcher.
func TestBotBotIRC(t *testing.T) {
	common.SetGlogFlags()

	// Create a mock storage with configuration in it
	storage := common.NewMockStorage(SERVER_PORT)

	// Create a mock queue to gather BotBot output
	queue := common.NewMockQueue()

	// Start a Mock IRC server, and gather writes to it
	server := common.NewMockIRCServer(TEST_MSG, SERVER_PORT)
	go server.Run(t)

	// Run BotBot
	time.Sleep(time.Second) // Sleep of one second to avoid the 5s backoff
	botbot := NewBotBot(storage, queue)
	go botbot.listen("testcmds")
	go botbot.mainLoop()
	waitForServer(server, 4)

	// this sleep allow us to keep the answer in the right order
	time.Sleep(time.Second)
	// Test sending a reply - should probably be separate test
	queue.ReadChannel <- "WRITE 1 #unit I am a plugin response"
	waitForServer(server, 6)

	tries := 0
	queue.RLock()
	q := queue.Got["q"]
	queue.RUnlock()

	for len(q) < 4 && tries < 4 {
		queue.RLock()
		q = queue.Got["q"]
		queue.RUnlock()

		glog.V(4).Infoln("[Debug] queue.Got[\"q\"]", len(q), "/", 4, q)
		time.Sleep(time.Second)
		tries++
	}
	checkContains(q, TEST_MSG, t)

	// Check IRC server expectations

	if server.GotLength() != 7 {
		t.Fatal("Expected exactly 7 IRC messages from the bot. Got ", server.GotLength())
	}

	glog.Infoln("[Debug] server.Got", server.Got)
	expect := []string{"PING", "CAP", "USER", "NICK", "NickServ", "JOIN", "PRIVMSG"}
	for i := 0; i < 5; i++ {
		if !strings.Contains(string(server.Got[i]), expect[i]) {
			t.Error("Line ", i, " did not contain ", expect[i], ". It is: ", server.Got[i])
		}
	}

	// test shutdown - should probably be separate test

	botbot.shutdown()

	tries = 0
	val := 5
	for len(q) < val && tries < val {
		queue.RLock()
		q = queue.Got["q"]
		queue.RUnlock()
		glog.V(4).Infoln("[Debug] queue.Got[\"q\"]", len(q), "/", val, q)
		time.Sleep(time.Second)
		tries++
	}

	queue.RLock()
	checkContains(queue.Got["q"], "SHUTDOWN", t)
	queue.RUnlock()
}

// Block until len(target.Get) is at least val, or timeout
func waitForServer(target *common.MockIRCServer, val int) {
	tries := 0
	for target.GotLength() < val && tries < val*3 {
		time.Sleep(time.Millisecond * 500)
		glog.V(4).Infoln("[Debug] val", target.GotLength(), "/", val, " target.Got:", target.Got)
		tries++
	}
	glog.Infoln("[Debug] waitForServer val", target.GotLength(), "/", val, " target.Got:", target.Got)

}

// Check that "val" is in one of the strings in "arr". t.Error if not.
func checkContains(arr []string, val string, t *testing.T) {
	glog.Infoln("[Debug] checkContains", val, "in", arr)

	isFound := false
	for _, item := range arr {
		if strings.Contains(item, val) {
			isFound = true
			break
		}
	}
	if !isFound {
		t.Error("Queue did not get a message containing:", val)
	}
}
