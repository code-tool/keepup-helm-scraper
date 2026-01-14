FROM golang:1.25 AS builder
WORKDIR /opt/keepup/
COPY go.mod ./
COPY go.sum .
COPY src ./src
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o helm-scraper src/main.go

FROM alpine:latest
WORKDIR /opt/keepup/
COPY --from=builder /opt/keepup/helm-scraper .
RUN chmod +x /opt/keepup/helm-scraper

ENTRYPOINT ["/opt/keepup/helm-scraper"]
