package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/lincolnloop/botbot-bot/common"
)

const (
	// Prefix of Redis channel to listen for messages on
	LISTEN_QUEUE_PREFIX = "bot"
)

func main() {
	log.SetFlags(log.Lshortfile | log.Ldate | log.Ltime)
	log.Println("START. Use 'botbot -help' for command line options.")

	storage := common.NewPostgresStorage()
	defer storage.Close()

	queue := common.NewRedisQueue()

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
