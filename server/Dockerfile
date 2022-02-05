FROM golang:1.17 as go
WORKDIR /go/src/app
COPY . .
RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o ephemerain-linux-amd64

FROM alpine:latest as alpine
RUN apk add -U --no-cache ca-certificates

FROM scratch
COPY --from=alpine /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=go /go/src/app/ephemerain-linux-amd64 /ephemerain-linux-amd64

ENTRYPOINT [ "/ephemerain-linux-amd64" ]