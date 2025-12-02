FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY main.go .
RUN go mod init mock-toncenter && \
    go mod tidy && \
    go build -o /mock-toncenter main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates curl
COPY --from=builder /mock-toncenter /usr/local/bin/mock-toncenter

EXPOSE 8082
ENTRYPOINT ["/usr/local/bin/mock-toncenter"]
