package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	//"net/http"
	//	"os"
	"time"

	"github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	cli, err := client.NewEnvClient()
	if err != nil {
		panic(err)
	}

	options := types.ContainerListOptions{All: true}
	containers, err := cli.ContainerList(options)
	if err != nil {
		panic(err)
	}

	var container types.Container
	for _, c := range containers {
		for _, name := range c.Names {
			if strings.Contains(name, "slave-mysql") {
				container = c
				break
			}
		}
	}

	fmt.Printf("Found slave mysql at %v", container.Ports[0].IP)

	dsn := fmt.Sprintf("root:mysql@tcp(%s:3306)/docker?charset=utf8", container.Ports[0].IP)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}

	// Лочим таблицы
	if _, err := db.Exec(`FLUSH TABLES WITH READ LOCK;`); err != nil {
		log.Print(err)
	}

	// Сохраняем контейнер в новый образ == из diff'а файловой системы делаем новый слой
	commitOptions := types.ContainerCommitOptions{
		ContainerID:    container.ID,
		RepositoryName: "test-mysql-snapshot",
		Tag:            time.Now().Format("20060102-150405"),
		Comment:        "Snapshooter",
		Author:         "Snapshooter",
		Pause:          true,
	}
	response, err := cli.ContainerCommit(commitOptions)
	log.Printf("Commit response: %#v", response)
	log.Printf("Commit err: %v", err)

	// Разблокируем таблицы
	if _, err := db.Exec(`UNLOCK TABLES;`); err != nil {
		log.Print(err)
	}

	// http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
	// 	fmt.Fprintf(w, "Last time: %v", last.Unix())
	// })
	// log.Fatal(http.ListenAndServe(":8080", nil))
}
