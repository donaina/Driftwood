package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/callmidavid/apidiff/internal/proxy"
)

func main() {
	target := flag.String(
		"target",
		"http://localhost:3000",
		"Target API url",
	)

	port := flag.String(
		"port",
		"8787",
		"ApiDiff proxy port",
	)

	flag.Parse()

	p, err := proxy.New(*target)
	if err != nil {
		log.Fatal("failed to create proxy %v", err)
	}

	addr := ":" + *port

	log.Printf("ApiDiff is running on %s", addr)
	log.Printf("Forward requests to %s", *target)

	if err := http.ListenAndServe(addr, p.Handler()); err != nil {
		log.Fatal("server stopped %v", err)
	}
}
