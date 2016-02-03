package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	dsn := fmt.Sprintf("root:mysql@tcp(%s:3306)/docker?charset=utf8", os.Getenv("MYSQL_HOST"))
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}

	last := time.Now()

	go func() {
		ticker := time.NewTicker(time.Second)

		for {
			select {
			case last = <-ticker.C:
				_, err := db.Exec(
					`INSERT INTO repl_status (ts) VALUES (?)`,
					last.Format("2006-01-02 15:04:05"),
				)
				if err == nil {
					log.Printf("Inserted beacon for %v", last.Format("2006-01-02 15:04:05"))
				} else {
					log.Fatal(err)
				}
			}
		}
	}()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Last time: %v", last.Unix())
	})
	log.Fatal(http.ListenAndServe(":8080", nil))
}
