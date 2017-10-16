FROM golang:1.8.3 as golang
WORKDIR /go/src/github.com/openfaas/kafka-queue-worker/queue-worker

COPY vendor  vendor
COPY main.go  .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app .

FROM alpine:latest
#RUN apk add --no-cache ca-certificates
WORKDIR /root/

EXPOSE 8080
ENV http_proxy      ""
ENV https_proxy     ""

COPY --from=golang /go/src/github.com/openfaas/kafka-queue-worker/queue-worker/app    .

CMD ["./app"]
