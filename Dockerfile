FROM golang:1.6.0
MAINTAINER Yann Malet <yann.malet@gmail.com>

ADD . /go/src/github.com/BotBotMe/botbot-bot/
WORKDIR /go/src/github.com/BotBotMe/botbot-bot/

RUN go get -u github.com/kardianos/govendor

CMD go run main.go -logtostderr=true
