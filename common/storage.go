package common

import (
	"database/sql"
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/lib/pq"
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
	channels := make([]*Channel, 0)
	channels = append(channels, &Channel{Id: 1, Name: "#unit", Fingerprint: "5876HKJGYUT"})
	botConf := &BotConfig{Id: 1, Config: conf, Channels: channels}

	return &MockStorage{botConfs: []*BotConfig{botConf}}
}

func (ms *MockStorage) BotConfig() []*BotConfig {
	return ms.botConfs
}

func (ms *MockStorage) SetCount(channel string, count int) error {
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
	postgresUrlString := os.Getenv("STORAGE_URL")
	if glog.V(2) {
		glog.Infoln("postgresUrlString: ", postgresUrlString)
	}
	if postgresUrlString == "" {
		glog.Fatal("STORAGE_URL cannot be empty.\nexport STORAGE_URL=postgres://user:password@host:port/db_name")
	}
	dataSource, err := pq.ParseURL(postgresUrlString)
	if err != nil {
		glog.Fatal("Could not read database string", err)
	}
	db, err := sql.Open("postgres", dataSource+" sslmode=disable fallback_application_name=bot")
	if err != nil {
		glog.Fatal("Could not connect to database.", err)
	}

	// The following 2 lines mitigate the leak of postgresql connection leak
	// explicitly setting a maximum number of postgresql connections
	db.SetMaxOpenConns(10)
	// explicitly setting a maximum number of Idle postgresql connections
	db.SetMaxIdleConns(2)

	return &PostgresStorage{db}
}

func (ps *PostgresStorage) BotConfig() []*BotConfig {

	var err error
	var rows *sql.Rows

	configs := make([]*BotConfig, 0)

	sql := "SELECT id, server, server_password, nick, password, real_name, server_identifier FROM bots_chatbot WHERE is_active=true"
	rows, err = ps.db.Query(sql)
	if err != nil {
		glog.Fatal("Error running: ", sql, " ", err)
	}
	defer rows.Close()

	var chatbotId int
	var server, server_password, nick, password, real_name, server_identifier []byte

	for rows.Next() {
		rows.Scan(
			&chatbotId, &server, &server_password, &nick, &password,
			&real_name, &server_identifier)

		confMap := map[string]string{
			"server":            string(server),
			"server_password":   string(server_password),
			"nick":              string(nick),
			"password":          string(password),
			"realname":          string(real_name),
			"server_identifier": string(server_identifier),
		}

		config := &BotConfig{
			Id:       chatbotId,
			Config:   confMap,
			Channels: make([]*Channel, 0),
		}

		configs = append(configs, config)
		glog.Infoln("config.Id:", config.Id)
	}
	channelStmt, err := ps.db.Prepare("SELECT id, name, password, fingerprint FROM bots_channel WHERE status=$1 and chatbot_id=$2")
	if err != nil {
		glog.Fatal("[Error] Error while preparing the statements to retrieve the channel:", err)
	}
	defer channelStmt.Close()

	for i := range configs {
		config := configs[i]
		rows, err = channelStmt.Query("ACTIVE", config.Id)
		if err != nil {
			glog.Fatal("Error running:", err)
		}
		defer rows.Close()

		var channelId int
		var channelName, channelPwd, channelFingerprint string
		for rows.Next() {
			rows.Scan(&channelId, &channelName, &channelPwd, &channelFingerprint)
			config.Channels = append(config.Channels,
				&Channel{Id: channelId, Name: channelName,
					Pwd: channelPwd, Fingerprint: channelFingerprint})
		}
		glog.Infoln("config.Channel:", config.Channels)
	}

	return configs
}

func (ms *PostgresStorage) SetCount(channel string, count int) error {

	now := time.Now()
	hour := now.Hour()

	channelId, err := ms.channelId(channel)
	if err != nil {
		return err
	}

	// Write the count
	updateSQL := "UPDATE bots_usercount SET counts[$1] = $2 WHERE channel_id = $3 AND dt = $4"

	var res sql.Result
	res, err = ms.db.Exec(updateSQL, hour, count, channelId, now)
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

	_, err = ms.db.Exec(insSQL, channelId, now)
	if err != nil {
		return err
	}

	// Run the update again
	_, err = ms.db.Query(updateSQL, hour, count, channelId, now)
	if err != nil {
		return err
	}

	return nil
}

// The channel Id for a given channel name
func (ms *PostgresStorage) channelId(name string) (int, error) {

	var channelId int
	query := "SELECT id from bots_channel WHERE name = $1"

	rows, err := ms.db.Query(query, name)
	if err != nil {
		return -1, err
	}
	defer rows.Close()

	rows.Next()
	rows.Scan(&channelId)

	if rows.Next() {
		glog.Fatal("More than one result. "+
			"Same name channels on different nets not yet supported. ", query)
	}

	return channelId, nil
}

func (ms *PostgresStorage) Close() error {
	return ms.db.Close()
}
