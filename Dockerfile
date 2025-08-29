FROM golang:1.23-alpine AS gobuild
WORKDIR /build-dir
COPY go.mod .
COPY go.sum .
RUN go mod download all
COPY . .
RUN go build -o /tmp/bridge github.com/tonkeeper/bridge


FROM alpine AS bridge
COPY --from=gobuild /tmp/bridge /app/bridge
CMD ["/app/bridge"]


