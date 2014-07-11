package dispatch

import (
	"github.com/golang/glog"

	"github.com/BotBotMe/botbot-bot/common"
	"github.com/BotBotMe/botbot-bot/line"
)

const (
	// Prefix of Redis channel to publish messages on
	QUEUE_PREFIX   = "q"
	MAX_QUEUE_SIZE = 4096
)

type Dispatcher struct {
	queue common.Queue
}

func NewDispatcher(queue common.Queue) *Dispatcher {

	dis := &Dispatcher{queue: queue}
	return dis
}

// Main method - send the line to relevant plugins
func (self *Dispatcher) Dispatch(l *line.Line) {

	var err error
	err = self.queue.Rpush(QUEUE_PREFIX, l.AsJson())
	if err != nil {
		glog.Fatal("Error writing (RPUSH) to queue. ", err)
	}
	self.limitQueue(QUEUE_PREFIX)
}

// Ensure the redis queue doesn't exceed a certain size
func (self *Dispatcher) limitQueue(key string) {

	size, err := self.queue.Llen(key)
	if err != nil {
		glog.Fatal("Error LLEN on queue. ", err)
	}

	if size < MAX_QUEUE_SIZE {
		return
	}

	err = self.queue.Ltrim(key, 0, MAX_QUEUE_SIZE)
	if err != nil {
		glog.Fatal("Error LTRIM on queue. ", err)
	}
}

// Dispatch the line to several channels.
// We need this for QUIT for example, which goes to all channels
// that user was in.
func (self *Dispatcher) DispatchMany(l *line.Line, channels []string) {

	for _, chName := range channels {
		l.Channel = chName
		self.Dispatch(l)
	}
}
