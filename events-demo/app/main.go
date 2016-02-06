package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	port := "80"
	if os.Getenv("PORT") != "" {
		port = os.Getenv("PORT")
	}
	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalln(err)
		return
	}
	log.Printf("Starting app at %v", hostname)

	// Простой хендлер, который показывает хост и текущее время
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t := time.Now()

		fmt.Fprintf(w, "Hello from %v\nTime is %v\n",
			hostname, time.Now().Format("01.02.2006 15:04:05"))

		// Ну и access.log для наглядности
		log.Printf("%s %q %v\n", r.Method, r.URL.String(), time.Since(t))
	})
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
