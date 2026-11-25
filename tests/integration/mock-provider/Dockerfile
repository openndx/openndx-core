# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Lanka Software Foundation

FROM golang:1.24-alpine

WORKDIR /app
COPY main.go .
RUN go build -o server main.go

EXPOSE 8083

CMD ["./server"]
