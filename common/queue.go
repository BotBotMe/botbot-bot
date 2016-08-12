package common

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
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

	// Append item to the beginning (left) of a list. Creates the list if needed.
	Lpush(key string, val []byte) error

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
	sync.RWMutex
	Got         map[string][]string
	ReadChannel chan string
}

func NewMockQueue() *MockQueue {
	return &MockQueue{
		Got:         make(map[string][]string),
		ReadChannel: make(chan string),
	}
}

func (mq *MockQueue) Publish(queue string, message []byte) error {
	mq.Lock()
	defer mq.Unlock()
	mq.Got[queue] = append(mq.Got[queue], string(message))
	return nil
}

func (mq *MockQueue) Rpush(key string, val []byte) error {
	mq.Lock()
	defer mq.Unlock()
	mq.Got[key] = append(mq.Got[key], string(val))
	return nil
}

func (mq *MockQueue) Lpush(key string, val []byte) error {
	mq.Lock()
	defer mq.Unlock()
	// TODO insert at the beginning of the slice
	mq.Got[key] = append(mq.Got[key], string(val))
	return nil
}

func (mq *MockQueue) Blpop(keys []string, timeoutsecs uint) (*string, []byte, error) {
	val := <-mq.ReadChannel
	return &keys[0], []byte(val), nil
}

func (mq *MockQueue) Llen(key string) (int, error) {
	mq.RLock()
	defer mq.RUnlock()
	return len(mq.Got), nil
}

func (mq *MockQueue) Ltrim(key string, start int, end int) error {
	return nil
}

func (mq *MockQueue) Ping() (string, error) {
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
	redisUrlString := os.Getenv("REDIS_PLUGIN_QUEUE_URL")
	if redisUrlString == "" {

		// try to connect via docker links if they exist.
		if os.Getenv("REDIS_PORT_6379_TCP_ADDR") != "" {
			redisUrlString = fmt.Sprintf("redis://%s:%s/0",
				os.Getenv("REDIS_PORT_6379_TCP_ADDR"),
				os.Getenv("REDIS_PORT_6379_TCP_PORT"),
			)
		} else {
			glog.Fatal("REDIS_PLUGIN_QUEUE_URL cannot be empty.\nexport REDIS_PLUGIN_QUEUE_URL=redis://host:port/db_number")

		}
	}
	redisUrl, err := url.Parse(redisUrlString)
	if err != nil {
		glog.Fatal("Could not read Redis string", err)
	}

	redisDb, err := strconv.Atoi(strings.TrimLeft(redisUrl.Path, "/"))
	if err != nil {
		glog.Fatal("Could not read Redis path", err)
	}

	redisQueue := goredis.Client{Addr: redisUrl.Host, Db: redisDb}
	rq := RedisQueue{queue: &redisQueue}
	rq.waitForRedis()
	return &rq
}

func (rq *RedisQueue) waitForRedis() {

	_, err := rq.queue.Ping()
	for err != nil {
		glog.Errorln("Waiting for redis...")
		time.Sleep(1 * time.Second)

		_, err = rq.queue.Ping()
	}
}

func (rq *RedisQueue) Publish(queue string, message []byte) error {

	err := rq.queue.Publish(queue, message)
	if err == nil {
		return nil
	}

	netErr := err.(net.Error)
	if netErr.Timeout() || netErr.Temporary() {
		return err
	}

	rq.waitForRedis()
	return rq.Publish(queue, message) // Recurse
}

func (rq *RedisQueue) Blpop(keys []string, timeoutsecs uint) (*string, []byte, error) {

	key, val, err := rq.queue.Blpop(keys, timeoutsecs)
	if err == nil {
		return key, val, nil
	}

	netErr := err.(net.Error)
	if netErr.Timeout() || netErr.Temporary() {
		return key, val, err
	}

	rq.waitForRedis()
	return rq.Blpop(keys, timeoutsecs) // Recurse
}

func (rq *RedisQueue) Rpush(key string, val []byte) error {

	err := rq.queue.Rpush(key, val)
	if err == nil {
		return nil
	}

	netErr := err.(net.Error)
	if netErr.Timeout() || netErr.Temporary() {
		return err
	}

	rq.waitForRedis()
	return rq.Rpush(key, val) // Recurse
}

func (rq *RedisQueue) Lpush(key string, val []byte) error {

	err := rq.queue.Lpush(key, val)
	if err == nil {
		return nil
	}

	netErr := err.(net.Error)
	if netErr.Timeout() || netErr.Temporary() {
		return err
	}

	rq.waitForRedis()
	return rq.Lpush(key, val) // Recurse
}

func (rq *RedisQueue) Llen(key string) (int, error) {

	size, err := rq.queue.Llen(key)
	if err == nil {
		return size, nil
	}

	netErr := err.(net.Error)
	if netErr.Timeout() || netErr.Temporary() {
		return size, err
	}

	rq.waitForRedis()
	return rq.Llen(key) // Recurse
}

func (rq *RedisQueue) Ltrim(key string, start int, end int) error {

	err := rq.queue.Ltrim(key, start, end)
	if err == nil {
		return nil
	}

	netErr := err.(net.Error)
	if netErr.Timeout() || netErr.Temporary() {
		return err
	}

	rq.waitForRedis()
	return rq.Ltrim(key, start, end) // Recurse
}

func (rq *RedisQueue) Ping() (string, error) {
	return rq.queue.Ping()
}
