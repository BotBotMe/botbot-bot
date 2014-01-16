package common

// Storage. Wraps the database
type Storage interface {
	BotConfig() []*BotConfig
	SetCount(string, int) error
}

/*
 * Mock STORAGE
 */

// Simplistic Storage implementation used by the test suite
type MockStorage struct {
	botConfs []*BotConfig
}

func NewMockStorage(serverPort string) Storage {

	conf := map[string]string{
		"nick":     "test",
		"password": "testxyz",
		"server":   "127.0.0.1:" + serverPort}
	channels := make([]string, 0)
	channels = append(channels, "#unit")
	botConf := &BotConfig{Id: 1, Config: conf, Channels: channels}

	return &MockStorage{botConfs: []*BotConfig{botConf}}
}

func (self *MockStorage) BotConfig() []*BotConfig {
	return self.botConfs
}

func (self *MockStorage) SetCount(channel string, count int) error {
	return nil
}
