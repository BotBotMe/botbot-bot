package main

import (
	"github.com/lincolnloop/botbot-bot/common"
	"strings"
	"testing"
	"time"
)

const (
	SERVER_PORT = "60667"
	TEST_MSG    = "q: Something new"
)

// A serious integration test for BotBot.
// This covers BotBot, the IRC code, and the dispatcher.
func TestBotBotIRC(t *testing.T) {

	// Create a mock storage with configuration in it
	storage := common.NewMockStorage(SERVER_PORT)

	// Create a mock queue to gather BotBot output
	queue := common.NewMockQueue()

	// Start a Mock IRC server, and gather writes to it
	server := common.NewMockIRCServer(TEST_MSG, SERVER_PORT)
	go server.Run(t)

	// Run BotBot
	botbot := NewBotBot(storage, queue)
	go botbot.listen("testcmds")
	go botbot.mainLoop()
	waitForServer(server, 5)

	// Test sending a reply - should probably be separate test
	queue.ReadChannel <- "WRITE 1 #unit I am a plugin response"
	waitForServer(server, 6)

	checkContains(queue.Got["q"], TEST_MSG, t)

	// Check IRC server expectations

	if len(server.Got) != 6 {
		t.Fatal("Expected exactly 6 IRC messages from the bot. Got ", len(server.Got))
	}

	expect := []string{"PING", "USER", "NICK", "NickServ", "JOIN", "PRIVMSG"}
	for i := 0; i < 5; i++ {
		if !strings.Contains(string(server.Got[i]), expect[i]) {
			t.Error("Line ", i, " did not contain ", expect[i], ". It is: ", server.Got[i])
		}
	}

	// test shutdown - should probably be separate test

	botbot.shutdown()

	tries := 0
	for len(queue.Got["q"]) < 4 && tries < 20 {
		time.Sleep(50 * time.Millisecond)
		tries++
	}

	checkContains(queue.Got["q"], "SHUTDOWN", t)
}

// Block until len(target.Get) is at least val, or timeout
func waitForServer(target *common.MockIRCServer, val int) {

	tries := 0
	for len(target.Got) < val && tries < 30 {
		time.Sleep(50 * time.Millisecond)
		tries++
	}
}

// Check that "val" is in one of the strings in "arr". t.Error if not.
func checkContains(arr []string, val string, t *testing.T) {

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
