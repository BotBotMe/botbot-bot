package main

import (
	_ "expvar"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	_ "net/http/pprof"

	"github.com/BotBotMe/botbot-bot/common"
	"github.com/BotBotMe/botbot-bot/log"
)

const (
	// Prefix of Redis channel to listen for messages on
	LISTEN_QUEUE_PREFIX = "bot"
)

func main() {
	log.Log.Infoln("START. Use 'botbot -help' for command line options.")

	storage := common.NewPostgresStorage()
	defer storage.Close()

	queue := common.NewRedisQueue()

	botbot := NewBotBot(storage, queue)

	// Listen for incoming commands
	go botbot.listen(LISTEN_QUEUE_PREFIX)

	// Start the main loop
	go botbot.mainLoop()

	// Start and http server to serve the stats from expvar
	log.Log.Fatal(http.ListenAndServe(":3030", nil))

	// Trap stop signal (Ctrl-C, kill) to exit
	kill := make(chan os.Signal)
	signal.Notify(kill, syscall.SIGINT, syscall.SIGKILL, syscall.SIGTERM)

	// Wait for stop signal
	for {
		<-kill
		log.Log.Infoln("Graceful shutdown")
		botbot.shutdown()
		break
	}

	log.Log.Infoln("Bye")
}
