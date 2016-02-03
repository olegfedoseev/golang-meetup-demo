FROM golang:1.5.1-alpine

RUN apk add --update git
RUN go get -v -d github.com/go-sql-driver/mysql
WORKDIR /go/src/github.com/olegfedoseev/golang-meetup-demo/mysql-demo/mysql-prod

COPY . /go/src/github.com/olegfedoseev/golang-meetup-demo/mysql-demo/mysql-prod

RUN go build -o /mysql-prod github.com/olegfedoseev/golang-meetup-demo/mysql-demo/mysql-prod

ENTRYPOINT ["/mysql-prod"]
