package main

import (
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"

	"github.com/BotBotMe/botbot-bot/common"
	"github.com/BotBotMe/botbot-bot/dispatch"
	"github.com/BotBotMe/botbot-bot/line"
	"github.com/BotBotMe/botbot-bot/network"
	"github.com/BotBotMe/botbot-bot/user"
)

/*
 * BOTBOT - the main object
 */

type BotBot struct {
	netMan     *network.NetworkManager
	dis        *dispatch.Dispatcher
	users      *user.UserManager
	storage    common.Storage
	queue      common.Queue
	fromServer chan *line.Line
	fromBus    chan string
}

func NewBotBot(storage common.Storage, queue common.Queue) *BotBot {

	fromServer := make(chan *line.Line)
	fromBus := make(chan string)

	netMan := network.NewNetworkManager(storage, fromServer)
	netMan.RefreshChatbots()
	go netMan.MonitorChatbots()

	dis := dispatch.NewDispatcher(queue)

	users := user.NewUserManager()

	return &BotBot{
		netMan:     netMan,
		dis:        dis,
		users:      users,
		queue:      queue,
		storage:    storage,
		fromServer: fromServer,
		fromBus:    fromBus}
}

// Listen for incoming commands
func (self *BotBot) listen(queueName string) {

	var msg []byte
	var err error

	for {
		_, msg, err = self.queue.Blpop([]string{queueName}, 0)
		if err != nil {
			glog.Fatal("Error reading (BLPOP) from queue. ", err)
		}
		if len(msg) != 0 {
			if glog.V(1) {
				glog.Infoln("Command: ", string(msg))
			}
			self.fromBus <- string(msg)
		}
	}
}

func (self *BotBot) mainLoop() {
	// TODO (yml) comment out self.recordUserCounts because I think it is
	// leaking postgres connection.
	//go self.recordUserCounts()

	var busCommand string
	var args string
	for {
		select {
		case serverLine, ok := <-self.fromServer:
			if !ok {
				// Channel is closed, we're offline. Stop.
				break
			}

			switch serverLine.Command {

			// QUIT and NICK don't have a channel name
			// They need to go to all channels the user is in
			case "QUIT", "NICK":
				self.dis.DispatchMany(serverLine, self.users.In(serverLine.User))

			default:
				self.dis.Dispatch(serverLine)
			}

			self.users.Act(serverLine)

		case busMessage, ok := <-self.fromBus:
			if !ok {
				break
			}

			parts := strings.SplitN(busMessage, " ", 2)
			busCommand = parts[0]
			if len(parts) > 1 {
				args = parts[1]
			}

			self.handleCommand(busCommand, args)
		}
	}
}

// Handle a command send from a plugin.
// Current commands:
//  - WRITE <chatbotid> <channel> <msg>: Send message to server
//  - REFRESH: Reload plugin configuration
func (self *BotBot) handleCommand(cmd string, args string) {
	if glog.V(2) {
		glog.Infoln("HandleCommand:", cmd)
	}
	switch cmd {
	case "WRITE":
		parts := strings.SplitN(args, " ", 3)
		chatbotId, err := strconv.Atoi(parts[0])
		if err != nil {
			if glog.V(1) {
				glog.Errorln("Invalid chatbot id: ", parts[0])
			}
			return
		}

		self.netMan.Send(chatbotId, parts[1], parts[2])

		// Now send it back to ourself, so other plugins see it
		internalLine := &line.Line{
			ChatBotId: chatbotId,
			Raw:       args,
			User:      self.netMan.GetUserByChatbotId(chatbotId),
			Command:   "PRIVMSG",
			Received:  time.Now().UTC().Format(time.RFC3339Nano),
			Content:   parts[2],
			Channel:   strings.TrimSpace(parts[1])}

		self.dis.Dispatch(internalLine)

	case "REFRESH":
		if glog.V(1) {
			glog.Infoln("Reloading configuration from database")
		}
		self.netMan.RefreshChatbots()
	}
}

// Writes the number of users per channel, every hour. Run in go routine.
func (self *BotBot) recordUserCounts() {

	for {

		for ch, _ := range self.users.Channels() {
			self.storage.SetCount(ch, self.users.Count(ch))
		}
		time.Sleep(1 * time.Hour)
	}
}

// Stop
func (self *BotBot) shutdown() {
	self.netMan.Shutdown()
}
