package common

import (
	"database/sql"
	"log"
	"os"
	"time"

	"github.com/bmizerany/pq"
)

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

/*
 * POSTGRES STORAGE
 */

type PostgresStorage struct {
	db *sql.DB
}

// Connect to the database.
func NewPostgresStorage() *PostgresStorage {
	postgresUrlString := os.Getenv("DATABASE_URL")
	if postgresUrlString == "" {
		postgresUrlString = "postgres://localhost/botbot"
	}
	dataSource, err := pq.ParseURL(postgresUrlString)
	if err != nil {
		log.Fatal("Could not read database string", err)
	}
	db, err := sql.Open("postgres", dataSource+" sslmode=disable")
	if err != nil {
		log.Fatal("Could not connect to database.", err)
	}

	return &PostgresStorage{db}
}

func (self *PostgresStorage) BotConfig() []*BotConfig {

	var err error
	var rows *sql.Rows

	configs := make([]*BotConfig, 0)

	sql := "SELECT id, server, server_password, nick, password, real_name FROM bots_chatbot WHERE is_active=true"
	rows, err = self.db.Query(sql)
	if err != nil {
		log.Fatal("Error running: ", sql, " ", err)
	}

	var chatbotId int
	var server, server_password, nick, password, real_name []byte

	for rows.Next() {
		rows.Scan(&chatbotId, &server, &server_password, &nick, &password, &real_name)

		confMap := map[string]string{
			"server":          string(server),
			"server_password": string(server_password),
			"nick":            string(nick),
			"password":        string(password),
			"realname":        string(real_name),
		}

		config := &BotConfig{
			Id:       chatbotId,
			Config:   confMap,
			Channels: make([]string, 0),
		}

		configs = append(configs, config)
	}
	for i := range configs {
		config := configs[i]
		rows, err = self.db.Query("SELECT id, name, password FROM bots_channel WHERE is_active=true and chatbot_id=$1", config.Id)
		if err != nil {
			log.Fatal("Error running:", err)
		}
		var channelId int
		var channelName string
		var channelPwd string
		for rows.Next() {
			rows.Scan(&channelId, &channelName, &channelPwd)
			config.Channels = append(config.Channels, channelName+" "+channelPwd)
		}
		log.Println("config.Channel:", config.Channels)
	}

	return configs
}

func (self *PostgresStorage) SetCount(channel string, count int) error {

	now := time.Now()
	hour := now.Hour()

	channelId, err := self.channelId(channel)
	if err != nil {
		return err
	}

	// Write the count

	updateSQL := "UPDATE bots_usercount SET counts[$1] = $2 WHERE channel_id = $3 AND dt = $4"

	var res sql.Result
	res, err = self.db.Exec(updateSQL, hour, count, channelId, now)
	if err != nil {
		return err
	}

	var rowCount int64
	rowCount, err = res.RowsAffected()
	if err != nil {
		return err
	}

	if rowCount == 1 {
		// Success - the update worked
		return nil
	}

	// Update failed, need to create the row first

	insSQL := "INSERT INTO bots_usercount (channel_id, dt, counts) VALUES ($1, $2, '{NULL}')"

	_, err = self.db.Exec(insSQL, channelId, now)
	if err != nil {
		return err
	}

	// Run the update again
	_, err = self.db.Query(updateSQL, hour, count, channelId, now)
	if err != nil {
		return err
	}

	return nil
}

// The channel Id for a given channel name
func (self *PostgresStorage) channelId(name string) (int, error) {

	var channelId int
	query := "SELECT id from bots_channel WHERE name = $1"

	rows, err := self.db.Query(query, name)
	if err != nil {
		return -1, err
	}

	rows.Next()
	rows.Scan(&channelId)

	if rows.Next() {
		log.Fatal("More than one result. "+
			"Same name channels on different nets not yet supported. ", query)
	}

	return channelId, nil
}

func (self *PostgresStorage) Close() error {
	return self.db.Close()
}
