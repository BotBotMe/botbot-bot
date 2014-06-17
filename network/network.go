package network

import (
	"sort"
	"sync"
	"time"

	"github.com/golang/glog"

	"github.com/BotBotMe/botbot-bot/common"
	"github.com/BotBotMe/botbot-bot/line"
	"github.com/BotBotMe/botbot-bot/network/irc"
)

type NetworkManager struct {
	sync.RWMutex
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

func (nm *NetworkManager) IsRunning() bool {
	nm.RLock()
	defer nm.RUnlock()
	return nm.isRunning
}

// Get the User for a ChatbotId
func (nm *NetworkManager) GetUserByChatbotId(id int) string {
	return nm.getChatbotById(id).GetUser()
}

// Connect to networks / start chatbots. Loads chatbot configuration from DB.
func (nm *NetworkManager) RefreshChatbots() {
	nm.Lock()
	defer nm.Unlock()
	if glog.V(2) {
		glog.Infoln("Entering in NetworkManager.RefreshChatbots")
	}
	botConfigs := nm.storage.BotConfig()

	var current common.ChatBot
	var id int
	active := make(sort.IntSlice, 0)

	// Create new ones
	for _, config := range botConfigs {
		id = config.Id
		active = append(active, id)

		current = nm.chatbots[id]
		if current == nil {
			// Create
			if glog.V(2) {
				glog.Infoln("Connect the bot with the following config:", config)
			}
			nm.chatbots[id] = nm.Connect(config)
		} else {
			// Update
			if glog.V(2) {
				glog.Infoln("Update the bot with the following config:", config)
			}
			nm.chatbots[id].Update(config)
		}

	}

	// Stop old ones
	active.Sort()
	numActive := len(active)

	for currId := range nm.chatbots {

		if active.Search(currId) == numActive { // if currId not in active:
			glog.Infoln("Stopping chatbot: ", currId)

			nm.chatbots[currId].Close()
			delete(nm.chatbots, currId)
		}
	}
	if glog.V(2) {
		glog.Infoln("Exiting NetworkManager.RefreshChatbots")
	}

}

func (nm *NetworkManager) Connect(config *common.BotConfig) common.ChatBot {

	glog.Infoln("Creating chatbot as:,", config)
	return irc.NewBot(config, nm.fromServer)
}

func (nm *NetworkManager) Send(chatbotId int, channel, msg string) {
	nm.RLock()
	nm.chatbots[chatbotId].Send(channel, msg)
	nm.RUnlock()
}

// Check out chatbots are alive, recreating them if not. Run this in go-routine.
func (nm *NetworkManager) MonitorChatbots() {

	for nm.IsRunning() {
		for id, bot := range nm.chatbots {
			if !bot.IsRunning() {
				nm.restart(id)
			}
		}
		time.Sleep(1 * time.Second)
	}
}

// get a chatbot by id
func (nm *NetworkManager) getChatbotById(id int) common.ChatBot {
	nm.RLock()
	defer nm.RUnlock()
	return nm.chatbots[id]
}

// Restart a chatbot
func (nm *NetworkManager) restart(botId int) {

	glog.Infoln("Restarting bot ", botId)

	var config *common.BotConfig

	// Find configuration for this bot

	botConfigs := nm.storage.BotConfig()
	for _, botConf := range botConfigs {
		if botConf.Id == botId {
			config = botConf
			break
		}
	}

	if config == nil {
		glog.Infoln("Could not find configuration for bot ", botId, ". Bot will not run.")
		delete(nm.chatbots, botId)
		return
	}

	nm.Lock()
	nm.chatbots[botId] = nm.Connect(config)
	nm.Unlock()
}

// Stop all bots
func (nm *NetworkManager) Shutdown() {
	nm.Lock()
	nm.isRunning = false
	for _, bot := range nm.chatbots {
		bot.Close()
	}
	nm.Unlock()
}
