//go:build tools
// +build tools

// Package tools tracks dependencies for build tools
package tools

import (
	_ "github.com/bufbuild/buf/cmd/buf"
	_ "github.com/dave/jennifer/jen"
	_ "github.com/drewfead/proto-cli/cmd/gen"
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/urfave/cli/v3"
	_ "google.golang.org/grpc/cmd/protoc-gen-go-grpc"
	_ "google.golang.org/protobuf/cmd/protoc-gen-go"
	_ "google.golang.org/protobuf/compiler/protogen"
)
