# Benzhi builder Dockerfile (template A, pure Go + go.sum).
FROM docker.m.daocloud.io/library/golang:1.26.3-bookworm

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build ./...
CMD ["bash"]
