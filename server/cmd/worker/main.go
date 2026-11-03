package main

import (
	"context"

	"linq-bs/server/internal/bootstrap"
)

func main() {
	app := bootstrap.NewWorker()
	if err := app.Run(context.Background()); err != nil {
		panic(err)
	}
}
