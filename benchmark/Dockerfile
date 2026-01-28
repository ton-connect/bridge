FROM golang:1.25-alpine

# Install build dependencies and utilities
RUN apk add --no-cache git build-base ca-certificates wget

# Build k6 with SSE extension
RUN go install go.k6.io/xk6/cmd/xk6@latest && \
    /go/bin/xk6 build --with github.com/phymbert/xk6-sse@latest -o /usr/local/bin/k6

# Working directory for tests
WORKDIR /scripts

# Copy test script
COPY bridge_test.js /scripts/bridge_test.js

# Simple entrypoint: just run k6, all env (including CURRENT_INSTANCE) is passed from outside
ENTRYPOINT ["k6"]
CMD ["run", "/scripts/bridge_test.js"]
