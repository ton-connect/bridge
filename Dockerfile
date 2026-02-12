FROM golang:1.26-alpine AS gobuild
RUN apk add --no-cache make git
WORKDIR /build-dir
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY . .
RUN make build && cp bridge /tmp/bridge


FROM alpine:latest AS bridge
COPY --from=gobuild /tmp/bridge /app/bridge
CMD ["/app/bridge"]
