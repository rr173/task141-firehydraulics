# Benzhi evaluation Dockerfile for task141-firehydraulics.
# Pure-Go project with go.sum; the native HTML/CSS/JS frontend is embedded via
# //go:embed so no Node build is needed. Template A (Go + go.sum, no Node).
FROM golang:1.26.3

WORKDIR /app

# Go dependencies (cached layer).
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Pre-compile once; the compile cache stays in the image. Does not affect edits.
RUN go build ./...
# `go build ./...` compiles packages into the cache. Build the service binary
# as well so the mandated container smoke command has an executable target.
RUN go build -o /app/firehydraulics .

# The evaluation command is `docker run <image> --smoke-test`; preserve the
# normal shell CMD while resolving that argument to the project's self-check.
RUN printf '#!/bin/sh\nexec /app/firehydraulics --smoke-test\n' > /usr/local/bin/--smoke-test \
    && chmod +x /usr/local/bin/--smoke-test

# Container starts in a shell for evaluation.
CMD ["bash"]
