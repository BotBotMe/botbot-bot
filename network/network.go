package network

import (
	"sort"
	"time"

	"github.com/golang/glog"

	"github.com/BotBotMe/botbot-bot/common"
	"github.com/BotBotMe/botbot-bot/line"
	"github.com/BotBotMe/botbot-bot/network/irc"
)

type NetworkManager struct {
	chatbots   map[int]common.ChatBot
	fromServer chan *line.Line
	storage    common.Storage
	isRunning  bool
}

func NewNetworkManager(storage common.Storage, fromServer chan *line.Line) *NetworkManager {

	netMan := &NetworkManager{
		chatbots:   make(map[int]common.ChatBot),
		fromServer: fromServer,
		storage:    storage,
		isRunning:  true,
	}

	return netMan
}

// Get the User for a ChatbotId
func (self *NetworkManager) GetUserByChatbotId(id int) string {
	return self.getChatbotById(id).GetUser()
}

// Connect to networks / start chatbots. Loads chatbot configuration from DB.
func (self *NetworkManager) RefreshChatbots() {
	if glog.V(2) {
		glog.Infoln("Entering in NetworkManager.RefreshChatbots")
	}
	botConfigs := self.storage.BotConfig()

	var current common.ChatBot
	var id int
	active := make(sort.IntSlice, 0)

	// Create new ones
	for _, config := range botConfigs {
		id = config.Id
		active = append(active, id)

		current = self.chatbots[id]
		if current == nil {
			// Create
			if glog.V(2) {
				glog.Infoln("Connect the bot with the following config:", config)
			}
			self.chatbots[id] = self.Connect(config)
		} else {
			// Update
			if glog.V(2) {
				glog.Infoln("Update the bot with the following config:", config)
			}
			self.chatbots[id].Update(config)
		}

	}

	// Stop old ones

	active.Sort()
	numActive := len(active)

	for currId := range self.chatbots {

		if active.Search(currId) == numActive { // if currId not in active:
			glog.Infoln("Stopping chatbot: ", currId)

			self.chatbots[currId].Close()
			delete(self.chatbots, currId)
		}
	}
	if glog.V(2) {
		glog.Infoln("Exiting NetworkManager.RefreshChatbots")
	}

}

func (self *NetworkManager) Connect(config *common.BotConfig) common.ChatBot {

	glog.Infoln("Creating chatbot,", config)
	return irc.NewBot(config, self.fromServer)
}

func (self *NetworkManager) Send(chatbotId int, channel, msg string) {
	self.chatbots[chatbotId].Send(channel, msg)
}

// Check out chatbots are alive, recreating them if not. Run this in go-routine.
func (self *NetworkManager) MonitorChatbots() {

	for self.isRunning {
		for id, bot := range self.chatbots {
			if !bot.IsRunning() {
				self.restart(id)
			}
		}
		time.Sleep(1 * time.Second)
	}
}

// get a chatbot by id
func (self *NetworkManager) getChatbotById(id int) common.ChatBot {
	return self.chatbots[id]
}

// Restart a chatbot
func (self *NetworkManager) restart(botId int) {

	glog.Infoln("Restarting bot ", botId)

	var config *common.BotConfig

	// Find configuration for this bot

	botConfigs := self.storage.BotConfig()
	for _, botConf := range botConfigs {
		if botConf.Id == botId {
			config = botConf
			break
		}
	}

	if config == nil {
		glog.Infoln("Could not find configuration for bot ", botId, ". Bot will not run.")
		delete(self.chatbots, botId)
		return
	}

	self.chatbots[botId] = self.Connect(config)
}

// Stop all bots
func (self *NetworkManager) Shutdown() {
	self.isRunning = false
	for _, bot := range self.chatbots {
		bot.Close()
	}
}
