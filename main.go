package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/muchzill4/rah-go/server"
)

func main() {
	srv := server.New()

	addr := ":8080"
	fmt.Printf("Listening on http://localhost%s\n", addr)
	log.Fatal(http.ListenAndServe(addr, srv))
}
