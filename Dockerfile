FROM golang:1.21.5-bullseye as builder

RUN mkdir /code

WORKDIR /code

RUN mkdir target

# Copy go.mod for downloading the dependencies before copying code to avoid downloading them on each build
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .

RUN go build -v -o /usr/local/bin/web ./cmd/web
RUN go build -v -o /usr/local/bin/taskmgr ./cmd/taskmgr

# Using multistage build to keep the image size minimal
FROM golang:1.21.5-bullseye as final

COPY --from=builder /usr/local/bin/web /usr/local/bin/taskmgr /usr/local/bin/