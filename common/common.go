package common

import (
	"io"
)

// Interface all bots must implement
type ChatBot interface {
	io.Closer
	Send(channel, msg string)
	Update(config *BotConfig)
	IsRunning() bool
	GetUser() string
}

// Configuration for a 'chatbot', which is what clients pay for
type BotConfig struct {
	Id     int
	Config map[string]string

	// Normally this array is names of the channels to join
	// but if the channel has a password, the string is "<channel> <pw>"
	// That means we can pass the string straight to JOIN and it works
	Channels []string
}

// Message queue
type Queue interface {

	// Publish 'message' on 'queue' (Redis calls it 'channel')
	Publish(queue string, message []byte) error

	// Append item to the end (right) of a list. Creates the list if needed.
	Rpush(key string, val []byte) error

	// Blocking Pop from one or more Redis lists
	Blpop(keys []string, timeoutsecs uint) (*string, []byte, error)

	// Check if queue is available. First return arg is "PONG".
	Ping() (string, error)

	// List length
	Llen(string) (int, error)

	// Trim list to given range
	Ltrim(string, int, int) error
}

// Storage. Wraps the database
type Storage interface {
	BotConfig() []*BotConfig
	SetCount(string, int) error
}
