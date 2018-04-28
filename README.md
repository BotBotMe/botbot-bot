[![Build Status](https://img.shields.io/circleci/project/github/BotBotMe/botbot-bot.svg)](https://circleci.com/gh/BotBotMe/botbot-bot)


The bot used in botbot.me is a Go (1.2+) program. To install:

    go get github.com/BotBotMe/botbot-bot

External resources:

* A Postgres database with the schema as defined in `schema.sql`.
* A Redis database used as a message bus between the plugins and the bot.

Before loading the sample data from `botbot_sample.dump` you will need to update the script with the irc nick and password.
Installing the database schema and loading sample data:

    psql -U botbot -h localhost -W botbot -f schema.sql
    psql -U botbot -h localhost -W botbot -f botbot_sample.dump

Configuration is handled via environment variables:

    STORAGE_URL=postgres://user:password@host:port/db_name \
    QUEUE_URL=redis://host:port/db_number botbot-bot

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
