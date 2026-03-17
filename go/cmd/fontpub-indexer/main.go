package main

import (
	"log"
	"net/http"
	"os"

	"github.com/ma/fontpub/go/internal/indexer/updateapi"
)

func main() {
	addr := os.Getenv("FONTPUB_INDEXER_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	server := updateapi.Server{
		Verifier:  updateapi.StaticVerifier{},
		Processor: updateapi.NotImplementedProcessor{},
	}

	log.Fatal(http.ListenAndServe(addr, server.Handler()))
}
