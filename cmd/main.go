package main

import (
	"log"

	"kube2e/internal/command"
)

func main() {
	if err := command.Run(); err != nil {
		log.Fatal(err)
	}
}
