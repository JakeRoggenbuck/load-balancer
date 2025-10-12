FROM golang:1.25-rc-bookworm

WORKDIR /app

COPY . .

EXPOSE 8080

CMD ["go", "run", "."]
