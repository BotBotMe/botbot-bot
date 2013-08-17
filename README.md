The bot used in botbot.me. To install:

    go get github.com/lincolnloop/botbot-bot

External resources:

* A Postgres database with the schema as defined in `schema.sql`.
* A Redis database used as a message bus between the plugins and the bot.

Configuration is handled via environment variables:

    DATABASE_URL=postgres://user:password@host:port/db_name \
    REDIS_PLUGIN_QUEUE_URL=redis://host:port/db_number botbot-bot

## Architecture

Execution starts in `main.go`, in function `main`. That starts the chatbots (via `NetworkManager`), the goroutine which listens for commands from Redis, and the `mainLoop` goroutine, then waits for a Ctrl-C or kill to quit.

The core of the bot is in `mainLoop` (`main.go`). That listens to two Go channels, `fromServer` and `fromBus`. `fromServer` receives everything coming in from IRC. `fromBus` receives commands from the plugins, sent via a Redis list.

A typical incoming request to a plugin would take this path:

```
IRC -> TCP socket -> ChatBot.listen (irc.go) -> fromServer channel -> mainLoop (main.go) -> Dispatcher (dispatch.go) -> redis PUBLISH -> plugin
```

A reply from the plugin takes this path:

```
plugin -> redis LPUSH -> listenCmd (main.go) -> fromBus channel -> mainLoop (main.go) -> NetworkManager.Send (network.go) -> ChatBot.Send (irc.go) -> TCP socket -> IRC
```

And now, in ASCII art:

```
plugins <--> REDIS -BLPOP-> listenCmd (main.go) --> fromBus --> mainLoop (main.go) <-- fromServer <-- n ChatBots (irc.go) <--> IRC
               ^                                                  | |                                      ^
               | PUBLISH                                          | |                                      |
                ------------ Dispatcher (dispatch.go) <----------   ----> NetworkManager (network.go) ----
```
