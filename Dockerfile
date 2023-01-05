FROM golang:1.19 as builder

WORKDIR /go/src/app
ADD . /go/src/app

RUN go test ./...
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -mod=vendor -a -installsuffix cgo \
    -o /go/bin/app .

FROM nginx:alpine

ADD output/ /usr/share/nginx/html/
