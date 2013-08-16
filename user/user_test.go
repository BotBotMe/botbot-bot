package user

import (
	"github.com/lincolnloop/botbot-bot/line"
	"testing"
)

// Add a nick to user manager
func TestAdd(t *testing.T) {

	u := NewUserManager()
	u.add("bob", "test")
	u.add("bob", "test2")
	u.add("bill", "test")

	_, ok := u.channels["test"]
	if !ok {
		t.Error("UserManager did not have channel")
	}

	if !has(u, "test", "bob") {
		t.Error("Channel test did not have bob")
	}
	if !has(u, "test", "bill") {
		t.Error("Channel test did not have bill")
	}
	if !has(u, "test2", "bob") {
		t.Error("Channel test2 did not have bob")
	}
}

// Remove a nick from user manager
func TestRemove(t *testing.T) {

	u := NewUserManager()
	u.add("bob", "test")
	u.add("bob", "test2")

	u.remove("bob")

	if has(u, "test", "bob") {
		t.Error("User not removed from test")
	}

	if has(u, "test2", "bob") {
		t.Error("User not removed from test2")
	}
}

// User changed their nick
func TestReplace(t *testing.T) {

	u := NewUserManager()
	u.add("bob", "test")
	u.add("bob", "test2")
	u.add("bill", "test")

	u.replace("bob", "bob|away")

	if has(u, "test", "bob") {
		t.Error("bob was not renamed in test")
	}
	if !has(u, "test", "bob|away") {
		t.Error("bob|away not found in test")
	}

	if has(u, "test2", "bob") {
		t.Error("bob was not renamed in test2")
	}
	if !has(u, "test2", "bob|away") {
		t.Error("bob|away not found in test2")
	}

	if !has(u, "test", "bill") {
		t.Error("bill was renamed or removed")
	}
}

// Add several users at once
func TestAddAct(t *testing.T) {

	l := line.Line{
		Command: "353",
		Content: "@alice bob charles",
		Channel: "test"}

	u := NewUserManager()
	u.Act(&l)

	if !has(u, "test", "alice") {
		t.Error("alice missing")
	}
	if !has(u, "test", "bob") {
		t.Error("bob missing")
	}
	if !has(u, "test", "charles") {
		t.Error("charles missing")
	}
}

// List of channels a user is in
func TestIn(t *testing.T) {

	u := NewUserManager()
	u.add("bob", "test")
	u.add("bob", "test2")
	u.add("bill", "test3")

	c := u.In("bob")

	if len(c) != 2 {
		t.Error("bob was not in exactly 2 channels")
	}

	if !(c[0] == "test" || c[1] == "test") {
		t.Error("test not in channel list")
	}
	if !(c[0] == "test2" || c[1] == "test2") {
		t.Error("test2 not in channel list")
	}
}

// Count the number of users in a channel
func TestCount(t *testing.T) {

	u := NewUserManager()
	u.add("bob", "test")
	u.add("bob", "test2")
	u.add("bill", "test")
	u.add("bill", "test3")

	if u.Count("test") != 2 {
		t.Error("#test should have 2 users")
	}
	if u.Count("test2") != 1 {
		t.Error("#test2 should have 1 user")
	}
	if u.Count("test3") != 1 {
		t.Error("#test3 should have 1 user")
	}
}

// utility
func has(u *UserManager, channel, nick string) bool {
	_, ok := u.channels[channel].users[nick]
	return ok
}
