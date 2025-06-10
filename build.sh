#!/bin/bash
version=$(git describe --tags --abbrev=0)
go build -ldflags "-X main.version=$version" ./cmd/grimux
