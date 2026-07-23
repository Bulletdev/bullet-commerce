FROM golang:1.23-alpine

WORKDIR /app
COPY . .
RUN go build -mod=vendor -o main cmd/main.go

EXPOSE 4444
CMD ["./main"]
