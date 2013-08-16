package user

import (
	"botbot-bot/line"
	"strings"
)

/*
 * USER MANAGER
 */

type UserManager struct {
	channels map[string]umChannel
}

// User Manager Channel - what user manager needs to know about a channel
type umChannel struct {
	users map[string]bool
}

func NewUserManager() *UserManager {

	channels := make(map[string]umChannel, 10) // 10 is just a hint to Go

	users := UserManager{channels}
	return &users
}

// Remember which users are in which channels.
// Call this on every server line.
func (self *UserManager) Act(line *line.Line) {

	switch line.Command {

	case "NICK":
		oldNick := line.User
		newNick := line.Content
		self.replace(oldNick, newNick)

	case "JOIN":
		self.add(line.User, line.Channel)

	case "PART":
		self.remove(line.User)

	case "QUIT":
		self.remove(line.User)

	case "353": // Reply to /names
		content := line.Content
		for _, nick := range strings.Split(content, " ") {
			nick = strings.Trim(nick, "@+")
			self.add(nick, line.Channel)
		}
	}

}

// Number of current users in the channel
func (self *UserManager) Count(channel string) int {

	ch, ok := self.channels[channel]
	if !ok {
		return 0
	}
	return len(ch.users)
}

// List of channels nick is in
func (self *UserManager) In(nick string) []string {

	var res []string

	for name, ch := range self.channels {

		_, ok := ch.users[nick]
		if ok {
			res = append(res, name)
		}
	}

	return res
}

// All the channels we have user counts for, as keys in a map
func (self *UserManager) Channels() map[string]umChannel {
	return self.channels
}

// Add user to channel
func (self *UserManager) add(nick, channel string) {

	ch, ok := self.channels[channel]
	if !ok { // First user for that channel
		ch = umChannel{make(map[string]bool)}
		self.channels[channel] = ch
	}

	ch.users[nick] = true
}

// Remove user from all channels
func (self *UserManager) remove(nick string) {

	for _, ch := range self.channels {
		delete(ch.users, nick) // If nick not in map, delete does nothing
	}
}

// Replace oldNick in every channel with newNick
func (self *UserManager) replace(oldNick, newNick string) {

	for _, ch := range self.channels {

		_, ok := ch.users[oldNick]
		if ok {
			delete(ch.users, oldNick)
			ch.users[newNick] = true
		}
	}
}
