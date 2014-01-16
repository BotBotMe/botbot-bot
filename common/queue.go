package common

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
