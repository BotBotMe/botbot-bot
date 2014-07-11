package dispatch

import (
	"github.com/BotBotMe/botbot-bot/common"
	"github.com/BotBotMe/botbot-bot/line"
	"testing"
)

func TestDispatch(t *testing.T) {

	dis := &Dispatcher{}
	l := line.Line{
		ChatBotId: 12,
		Command:   "PRIVMSG",
		Content:   "Hello",
		Channel:   "#foo"}

	queue := common.NewMockQueue()
	dis.queue = queue

	dis.Dispatch(&l)

	if len(queue.Got) != 1 {
		t.Error("Dispatch did not go on queue")
	}

	received, _ := queue.Got[QUEUE_PREFIX]
	if received == nil {
		t.Error("Expected '", QUEUE_PREFIX, "' as queue name")
	}
}
