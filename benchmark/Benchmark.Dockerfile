FROM golang:1.24-alpine AS build
RUN apk add --no-cache git build-base \
 && go install go.k6.io/xk6/cmd/xk6@latest \
 && /go/bin/xk6 build --with github.com/phymbert/xk6-sse@latest -o /k6 \
 && go install github.com/google/pprof@latest

FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget
COPY --from=build /k6 /usr/local/bin/k6
COPY --from=build /go/bin/pprof /usr/local/bin/pprof
WORKDIR /scripts
ENTRYPOINT ["/bin/sh","-lc"]