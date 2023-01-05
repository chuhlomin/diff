FROM golang:1.19 as builder

WORKDIR /src/
ADD . /src/

RUN go test ./...
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -mod=vendor -a -installsuffix cgo \
    -o /src/app .

RUN /src/app


FROM nginx:alpine
COPY --from=builder /src/output/ /usr/share/nginx/html/
