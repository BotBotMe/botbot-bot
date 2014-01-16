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
