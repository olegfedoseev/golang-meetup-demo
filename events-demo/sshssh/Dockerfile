FROM golang:1.5.1-alpine

RUN apk add --update git
RUN go get -v -d github.com/fsouza/go-dockerclient && \
	go get -v -d golang.org/x/crypto/ssh

ENV DOCKER_TLS_VERIFY 1
ENV DOCKER_CERT_PATH /etc/docker

WORKDIR /go/src/github.com/olegfedoseev/golang-meetup-demo/events-demo/sshssh
COPY . /go/src/github.com/olegfedoseev/golang-meetup-demo/events-demo/sshssh

RUN go build -o /sshssh github.com/olegfedoseev/golang-meetup-demo/events-demo/sshssh

EXPOSE 2222
ENTRYPOINT ["/sshssh"]

# docker run -d -P \
#	-e constraint:instance==frontend \
#   -e DOCKER_HOST=$DOCKER_HOST \
#	-v $DOCKER_CERT_PATH:/etc/docker sshssh
