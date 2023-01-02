FROM golang:1.19 as builder

WORKDIR /go/src/app
ADD . /go/src/app

RUN go test ./...
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -mod=vendor -a -installsuffix cgo \
    -o /go/bin/app .

FROM gcr.io/distroless/static:966f4bd97f611354c4ad829f1ed298df9386c2ec
# latest-amd64 -> 966f4bd97f611354c4ad829f1ed298df9386c2ec
# https://github.com/GoogleContainerTools/distroless/tree/master/base

COPY templates /templates
COPY static /static
COPY --from=builder /go/bin/app /app
ENTRYPOINT ["/app"]
