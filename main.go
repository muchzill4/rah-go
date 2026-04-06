package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/muchzill4/rah-go/server"
)

func main() {
	srv := server.New()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port
	fmt.Printf("Listening on http://localhost%s\n", addr)
	log.Fatal(http.ListenAndServe(addr, srv))
}
