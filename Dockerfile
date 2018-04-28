FROM golang

WORKDIR /go/src/github.com/BotBotMe/botbot-bot
COPY . .
RUN go get -v . && go build -v -o .

CMD ["./botbot-bot"]
