// Package web defines repository-level generation commands.
package web

//go:generate go run github.com/swaggo/swag/cmd/swag@v1.16.6 fmt -g cmd/server/main.go
//go:generate go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g cmd/server/main.go -o docs --parseInternal --requiredByDefault
