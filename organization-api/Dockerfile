FROM golang:1.23.4-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /organization-api .

FROM scratch
COPY --from=builder /organization-api /organization-api
ENTRYPOINT ["/organization-api"]
