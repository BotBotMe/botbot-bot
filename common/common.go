package common

import (
	"io"
	"strings"
)

// Interface all bots must implement
type ChatBot interface {
	io.Closer
	Send(channel, msg string)
	Update(config *BotConfig)
	//IsRunning() bool
	GetUser() string
}

// Configuration for a 'chatbot', which is what clients pay for
type BotConfig struct {
	Id       int
	Config   map[string]string
	Channels []*Channel
}

// Configuration for a channel
type Channel struct {
	Id          int
	Name        string
	Pwd         string
	Fingerprint string
}

func (cc *Channel) Credential() string {
	return strings.TrimSpace(cc.Name + " " + cc.Pwd)
}

func (cc *Channel) String() string {
	return cc.Name
}
