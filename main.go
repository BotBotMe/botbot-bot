package main

import (
	"net"
	nurl "net/url"
	"time"

	// third-party dependencies

	pq "github.com/bmizerany/pq"
	"github.com/monnand/goredis"
	"github.com/joho/godotenv"

	// local packages
	"github.com/lincolnloop/botbot-bot/common"
	"github.com/lincolnloop/botbot-bot/dispatch"
	"github.com/lincolnloop/botbot-bot/line"
	"github.com/lincolnloop/botbot-bot/network"
	"github.com/lincolnloop/botbot-bot/user"

	// stdlib package
	"database/sql"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

const (
	// Prefix of Redis channel to listen for messages on
	LISTEN_QUEUE_PREFIX   = "bot"
)

func main() {

	log.Println("START. Use 'botbot -help' for command line options.")
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	log.Println("Environment variables have been set from the .env file")
	log.Println("STORAGE_URL: ", os.Getenv("STORAGE_URL"))
	log.Println("REDIS_PLUGIN_QUEUE_URL: ", os.Getenv("REDIS_PLUGIN_QUEUE_URL"))

	storage := NewPostgresStorage()
	defer storage.Close()
	redisUrlString := os.Getenv("REDIS_PLUGIN_QUEUE_URL")
	if redisUrlString == "" {
		redisUrlString = "redis://localhost:6379/0"
	}
	redisUrl, err := nurl.Parse(redisUrlString)
	if err != nil {
		log.Fatal("Could not read Redis string", err)
	}
	redisQueue := goredis.Client{Addr: redisUrl.Host}
	queue := newReliableQueue(&redisQueue)

	botbot := NewBotBot(storage, queue)

	// Listen for incoming commands
	go botbot.listen(LISTEN_QUEUE_PREFIX)

	// Start the main loop
	go botbot.mainLoop()

	// Trap stop signal (Ctrl-C, kill) to exit
	kill := make(chan os.Signal)
	signal.Notify(kill, syscall.SIGINT, syscall.SIGKILL, syscall.SIGTERM)

	// Wait for stop signal
	for {
		<-kill
		log.Println("Graceful shutdown")
		botbot.shutdown()
		break
	}

	log.Println("Bye")
}

/*
 * BOTBOT - the main object
 */

type BotBot struct {
	netMan     *network.NetworkManager
	dis        *dispatch.Dispatcher
	users      *user.UserManager
	storage    common.Storage
	queue      common.Queue
	fromServer chan *line.Line
	fromBus    chan string
}

func NewBotBot(storage common.Storage, queue common.Queue) *BotBot {

	fromServer := make(chan *line.Line)
	fromBus := make(chan string)

	netMan := network.NewNetworkManager(storage, fromServer)
	netMan.RefreshChatbots()
	go netMan.MonitorChatbots()

	dis := dispatch.NewDispatcher(queue)

	users := user.NewUserManager()

	return &BotBot{
		netMan:     netMan,
		dis:        dis,
		users:      users,
		queue:      queue,
		storage:    storage,
		fromServer: fromServer,
		fromBus:    fromBus}
}

// Listen for incoming commands
func (self *BotBot) listen(queueName string) {

	var msg []byte
	var err error

	for {
		_, msg, err = self.queue.Blpop([]string{queueName}, 0)
		if err != nil {
			log.Fatal("Error reading (BLPOP) from queue. ", err)
		}
		if len(msg) != 0 {
			log.Println("Command: ", string(msg))
			self.fromBus <- string(msg)
		}
	}
}

func (self *BotBot) mainLoop() {

	go self.recordUserCounts()

	var busCommand string
	var args string
	for {
		select {

		case serverLine, ok := <-self.fromServer:
			if !ok {
				// Channel is closed, we're offline. Stop.
				break
			}

			switch serverLine.Command {

			// QUIT and NICK don't have a channel name
			// They need to go to all channels the user is in
			case "QUIT", "NICK":
				self.dis.DispatchMany(serverLine, self.users.In(serverLine.User))

			default:
				self.dis.Dispatch(serverLine)
			}

			self.users.Act(serverLine)

		case busMessage, ok := <-self.fromBus:
			if !ok {
				break
			}

			parts := strings.SplitN(busMessage, " ", 2)
			busCommand = parts[0]
			if len(parts) > 1 {
				args = parts[1]
			}

			self.handleCommand(busCommand, args)
		}
	}
}

// Handle a command send from a plugin.
// Current commands:
//  - WRITE <chatbotid> <channel> <msg>: Send message to server
//  - REFRESH: Reload plugin configuration
func (self *BotBot) handleCommand(cmd string, args string) {
	switch cmd {
	case "WRITE":
		parts := strings.SplitN(args, " ", 3)
		chatbotId, err := strconv.Atoi(parts[0])
		if err != nil {
			log.Println("Invalid chatbot id: ", parts[0])
			return
		}

		self.netMan.Send(chatbotId, parts[1], parts[2])

		// Now send it back to ourself, so other plugins see it
		internalLine := &line.Line{
			ChatBotId: chatbotId,
			Raw:       args,
			User:      self.netMan.GetUserByChatbotId(chatbotId),
			Command:   "PRIVMSG",
			Received:  time.Now().UTC().Format(time.RFC3339Nano),
			Content:   parts[2],
			Channel:   strings.TrimSpace(parts[1])}
		self.dis.Dispatch(internalLine)

	case "REFRESH":
		log.Println("Reloading configuration from database")
		self.netMan.RefreshChatbots()
	}
}

// Writes the number of users per channel, every hour. Run in go routine.
func (self *BotBot) recordUserCounts() {

	for {

		for ch, _ := range self.users.Channels() {
			self.storage.SetCount(ch, self.users.Count(ch))
		}
		time.Sleep(1 * time.Hour)
	}
}

// Stop
func (self *BotBot) shutdown() {
	self.netMan.Shutdown()
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

func (self *PostgresStorage) BotConfig() []*common.BotConfig {

	var err error
	var rows *sql.Rows

	configs := make([]*common.BotConfig, 0)

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

		config := &common.BotConfig{
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

/*
 * REDIS WRAPPER
 * Survives Redis restarts, waits for Redis to be available.
 * Implements common.Queue
 */
type reliableQueue struct {
	queue common.Queue
}

func newReliableQueue(queue common.Queue) common.Queue {
	s := reliableQueue{queue: queue}
	s.waitForRedis()
	return &s
}

func (self *reliableQueue) waitForRedis() {

	_, err := self.queue.Ping()
	for err != nil {
		log.Println("Waiting for redis...")
		time.Sleep(1 * time.Second)

		_, err = self.queue.Ping()
	}
}

func (self *reliableQueue) Publish(queue string, message []byte) error {

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

func (self *reliableQueue) Blpop(keys []string, timeoutsecs uint) (*string, []byte, error) {

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

func (self *reliableQueue) Rpush(key string, val []byte) error {

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

func (self *reliableQueue) Llen(key string) (int, error) {

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

func (self *reliableQueue) Ltrim(key string, start int, end int) error {

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

func (self *reliableQueue) Ping() (string, error) {
	return self.queue.Ping()
}
