// main.go (пример)
package main

import (
	"log"

	"wisp/config"
	"wisp/server"
)

func main() {
	cfg := config.MustLoad()
	app := &server.App{}
	app.Initialize(cfg)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
