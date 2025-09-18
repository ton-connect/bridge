FROM golang:1.24-alpine AS gobuild
WORKDIR /build-dir
COPY go.mod .
COPY go.sum .
RUN go mod download all
COPY . .
RUN go build -o /tmp/bridge github.com/tonkeeper/bridge/cmd/bridge


FROM scratch AS bridge
COPY --from=gobuild /tmp/bridge /app/bridge
CMD ["/app/bridge/cmd/bridge"]


