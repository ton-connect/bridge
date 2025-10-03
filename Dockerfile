FROM golang:1.24-alpine AS gobuild
WORKDIR /build-dir
COPY go.mod .
COPY go.sum .
RUN go mod download
ARG GIT_COMMIT
COPY . .
RUN go build -ldflags "-X main.GitCommit=$GIT_COMMIT" -o /tmp/bridge github.com/tonkeeper/bridge/cmd/bridge


FROM scratch AS bridge
COPY --from=gobuild /tmp/bridge /app/bridge
CMD ["/app/bridge"]
