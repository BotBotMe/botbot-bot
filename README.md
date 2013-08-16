The bot used in botbot.me. To build:

```
go get github.com/bmizerany/pq github.com/monnand/goredis
go install botbot-bot
```

External resources:

* A Postgres database with the schema as defined in `schema.sql`.
* A Redis database used as a message bus between the plugins and the bot.

Configuration is handled via environment variables:

    DATABASE_URL=postgres://user:password@host:port/db_name \
    REDIS_PLUGIN_QUEUE_URL=redis://host:port/db_number botbot-bot
