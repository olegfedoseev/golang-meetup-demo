FROM golang:1.5.3-alpine
MAINTAINER Oleg Fedoseev <oleg.fedoseev@me.com>

RUN apk add --update git && \
	go get -u github.com/fsouza/go-dockerclient

ENV DOCKER_TLS_VERIFY 1
ENV DOCKER_CERT_PATH /etc/docker

WORKDIR /go/src/github.com/olegfedoseev/golang-meetup-demo/events-demo/balancer
COPY . /go/src/github.com/olegfedoseev/golang-meetup-demo/events-demo/balancer
RUN go build -o /balancer github.com/olegfedoseev/golang-meetup-demo/events-demo/balancer

EXPOSE 80
ENTRYPOINT ["/balancer"]

# docker run -d -p 80:80 -e constraint:instance==frontend -e DOCKER_HOST=$DOCKER_HOST  -v $DOCKER_CERT_PATH:/etc/docker balancer
