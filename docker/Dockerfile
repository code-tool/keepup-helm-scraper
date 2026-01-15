FROM --platform=$BUILDPLATFORM golang:1.25 AS builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /opt/keepup/
COPY go.mod go.sum ./
COPY src ./src/
RUN go mod download
RUN CGO_ENABLED=0 \
    GOOS=$TARGETOS \
    GOARCH=$TARGETARCH \
    go build -a -installsuffix cgo -o helm-scraper src/main.go

FROM alpine:3
WORKDIR /opt/keepup/
COPY --from=builder /opt/keepup/helm-scraper .
RUN chmod +x /opt/keepup/helm-scraper
ENTRYPOINT ["/opt/keepup/helm-scraper"]
