FROM golang:1.5.3-alpine
MAINTAINER Oleg Fedoseev <oleg.fedoseev@me.com>

RUN apk add --update git

WORKDIR /go/src/github.com/olegfedoseev/golang-meetup-demo/events-demo/app
COPY . /go/src/github.com/olegfedoseev/golang-meetup-demo/events-demo/app
RUN go build -o /app github.com/olegfedoseev/golang-meetup-demo/events-demo/app

EXPOSE 80
ENTRYPOINT ["/app"]
