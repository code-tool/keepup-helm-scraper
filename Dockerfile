FROM --platform=$BUILDPLATFORM golang:1.25 AS builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /app
COPY main.go .
RUN go mod init helm-scraper && go mod tidy
RUN CGO_ENABLED=0 \
    GOOS=$TARGETOS \
    GOARCH=$TARGETARCH \
    go build -o helm-scraper main.go

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/helm-scraper /root/helm-scraper
RUN chmod +x /root/helm-scraper
ENTRYPOINT ["/root/helm-scraper"]
