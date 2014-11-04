FROM ubuntu:14.04
MAINTAINER Yann Malet <yann.malet@gmail.com>

RUN locale-gen en_US.UTF-8
ENV LANG en_US.UTF-8
ENV LANGUAGE en_US:en
ENV LC_ALL en_US.UTF-8
ENV PATH /usr/src/go/bin:$PATH
ENV GOPATH /go
ENV PATH /go/bin:$PATH
ENV GOLANG_VERSION 1.3.1


# SCMs for "go get", gcc for cgo
RUN DEBIAN_FRONTEND=noninteractive apt-get update
RUN DEBIAN_FRONTEND=noninteractive apt-get install -y \
    ca-certificates curl gcc libc6-dev \
    bzr git mercurial
RUN rm -rf /var/lib/apt/lists/*
RUN curl -sSL http://golang.org/dl/go$GOLANG_VERSION.src.tar.gz | tar -v -C /usr/src -xz

RUN cd /usr/src/go/src && ./make.bash --no-clean 2>&1

RUN mkdir -p /go/src
WORKDIR /go

ENV GOPACKAGE github.com/BotBotMe/botbot-bot
# Copy the local package files to the container's workspace.
ADD . /go/src/$GOPACKAGE

# Build the $GOPACKAGE command inside the container.
# (You may fetch or manage dependencies here,
# either manually or with a tool like "godep".)
RUN go get $GOPACKAGE

ENTRYPOINT /go/bin/botbot-bot
