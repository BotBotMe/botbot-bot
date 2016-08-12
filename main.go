package main

import (
	_ "expvar"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/BotBotMe/botbot-bot/botbot"
	"github.com/BotBotMe/botbot-bot/common"
	"github.com/golang/glog"
	_ "net/http/pprof"
)

const (
	// Prefix of Redis channel to listen for messages on
	LISTEN_QUEUE_PREFIX = "bot"
)

func main() {
	flag.Parse()
	glog.Infoln("START. Use 'botbot -help' for command line options.")

	storage := common.NewPostgresStorage()
	defer storage.Close()

	queue := common.NewRedisQueue()

	bot := botbot.NewBotBot(storage, queue)

	// Listen for incoming commands
	go bot.Listen(LISTEN_QUEUE_PREFIX)

	// Start the main loop
	go bot.MainLoop()

	// Start and http server to serve the stats from expvar
	log.Fatal(http.ListenAndServe(":3030", nil))

	// Trap stop signal (Ctrl-C, kill) to exit
	kill := make(chan os.Signal)
	signal.Notify(kill, syscall.SIGINT, syscall.SIGKILL, syscall.SIGTERM)

	// Wait for stop signal
	for {
		<-kill
		glog.Infoln("Graceful shutdown")
		bot.Shutdown()
		break
	}

	glog.Infoln("Bye")
}
