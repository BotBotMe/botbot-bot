package common

import (
	"net"
	"net/url"
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/monnand/goredis"
)

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

/*
 * Mock QUEUE
 */

// Simplistic Queue implementation used by the test suite
type MockQueue struct {
	Got         map[string][]string
	ReadChannel chan string
}

func NewMockQueue() *MockQueue {
	return &MockQueue{
		Got:         make(map[string][]string),
		ReadChannel: make(chan string),
	}
}

func (self *MockQueue) Publish(queue string, message []byte) error {
	self.Got[queue] = append(self.Got[queue], string(message))
	return nil
}

func (self *MockQueue) Rpush(key string, val []byte) error {
	self.Got[key] = append(self.Got[key], string(val))
	return nil
}

func (self *MockQueue) Blpop(keys []string, timeoutsecs uint) (*string, []byte, error) {
	val := <-self.ReadChannel
	return &keys[0], []byte(val), nil
}

func (self *MockQueue) Llen(key string) (int, error) {
	return len(self.Got), nil
}

func (self *MockQueue) Ltrim(key string, start int, end int) error {
	return nil
}

func (self *MockQueue) Ping() (string, error) {
	return "PONG", nil
}

/*
 * REDIS WRAPPER
 * Survives Redis restarts, waits for Redis to be available.
 * Implements common.Queue
 */
type RedisQueue struct {
	queue Queue
}

func NewRedisQueue() Queue {
	redisUrlString := os.Getenv("QUEUE_URL")
	if redisUrlString == "" {
		glog.Fatal("QUEUE_URL cannot be empty.\nexport QUEUE_URL=redis://host:port/db_number")
	}
	redisUrl, err := url.Parse(redisUrlString)
	if err != nil {
		glog.Fatal("Could not read Redis string", err)
	}
	redisQueue := goredis.Client{Addr: redisUrl.Host}
	s := RedisQueue{queue: &redisQueue}
	s.waitForRedis()
	return &s
}

func (self *RedisQueue) waitForRedis() {

	_, err := self.queue.Ping()
	for err != nil {
		glog.Errorln("Waiting for redis...")
		time.Sleep(1 * time.Second)

		_, err = self.queue.Ping()
	}
}

func (self *RedisQueue) Publish(queue string, message []byte) error {

	err := self.queue.Publish(queue, message)
	if err == nil {
		return nil
	}

	netErr := err.(net.Error)
	if netErr.Timeout() || netErr.Temporary() {
		return err
	}

	self.waitForRedis()
	return self.Publish(queue, message) // Recurse
}

func (self *RedisQueue) Blpop(keys []string, timeoutsecs uint) (*string, []byte, error) {

	key, val, err := self.queue.Blpop(keys, timeoutsecs)
	if err == nil {
		return key, val, nil
	}

	netErr := err.(net.Error)
	if netErr.Timeout() || netErr.Temporary() {
		return key, val, err
	}

	self.waitForRedis()
	return self.Blpop(keys, timeoutsecs) // Recurse
}

func (self *RedisQueue) Rpush(key string, val []byte) error {

	err := self.queue.Rpush(key, val)
	if err == nil {
		return nil
	}

	netErr := err.(net.Error)
	if netErr.Timeout() || netErr.Temporary() {
		return err
	}

	self.waitForRedis()
	return self.Rpush(key, val) // Recurse
}

func (self *RedisQueue) Llen(key string) (int, error) {

	size, err := self.queue.Llen(key)
	if err == nil {
		return size, nil
	}

	netErr := err.(net.Error)
	if netErr.Timeout() || netErr.Temporary() {
		return size, err
	}

	self.waitForRedis()
	return self.Llen(key) // Recurse
}

func (self *RedisQueue) Ltrim(key string, start int, end int) error {

	err := self.queue.Ltrim(key, start, end)
	if err == nil {
		return nil
	}

	netErr := err.(net.Error)
	if netErr.Timeout() || netErr.Temporary() {
		return err
	}

	self.waitForRedis()
	return self.Ltrim(key, start, end) // Recurse
}

func (self *RedisQueue) Ping() (string, error) {
	return self.queue.Ping()
}
