package main

import (
	"log"
	"os"
)

func main() {
	app, err := NewApp(os.Stdin, os.Stdout, os.Stderr, configPathFromEnv())
	if err != nil {
		log.Fatal(err)
	}

	os.Exit(app.Run(os.Args[1:]))
}
