package main

import (
	"time"

	// third-party dependencies

	// local packages
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"github.com/lincolnloop/botbot-bot/common"
	"github.com/lincolnloop/botbot-bot/dispatch"
	"github.com/lincolnloop/botbot-bot/line"
	"github.com/lincolnloop/botbot-bot/network"
	"github.com/lincolnloop/botbot-bot/user" // stdlib package
)

const (
	// Prefix of Redis channel to listen for messages on
	LISTEN_QUEUE_PREFIX = "bot"
)

func main() {

	log.Println("START. Use 'botbot -help' for command line options.")

	storage := common.NewPostgresStorage()
	defer storage.Close()

	queue := common.NewRedisQueue()

	botbot := NewBotBot(storage, queue)

	// Listen for incoming commands
	go botbot.listen(LISTEN_QUEUE_PREFIX)

	// Start the main loop
	go botbot.mainLoop()

	// Trap stop signal (Ctrl-C, kill) to exit
	kill := make(chan os.Signal)
	signal.Notify(kill, syscall.SIGINT, syscall.SIGKILL, syscall.SIGTERM)

	// Wait for stop signal
	for {
		<-kill
		log.Println("Graceful shutdown")
		botbot.shutdown()
		break
	}

	log.Println("Bye")
}

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
			log.Fatal("Error reading (BLPOP) from queue. ", err)
		}
		if len(msg) != 0 {
			log.Println("Command: ", string(msg))
			self.fromBus <- string(msg)
		}
	}
}

func (self *BotBot) mainLoop() {

	go self.recordUserCounts()

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
	switch cmd {
	case "WRITE":
		parts := strings.SplitN(args, " ", 3)
		chatbotId, err := strconv.Atoi(parts[0])
		if err != nil {
			log.Println("Invalid chatbot id: ", parts[0])
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
			Channel:   parts[1]}
		self.dis.Dispatch(internalLine)

	case "REFRESH":
		log.Println("Reloading configuration from database")
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
