package main

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"strings"
	"time"

	"github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"

	"github.com/go-sql-driver/mysql"
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

	fmt.Printf("Found MySQL slave server at %v\n", container.Ports[0].IP)

	var mysqlLog = mysql.Logger(log.New(ioutil.Discard, "", 0))

	dsn := fmt.Sprintf("root:mysql@tcp(%s:3306)/docker?charset=utf8", container.Ports[0].IP)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	mysql.SetLogger(mysqlLog)

	// Лочим таблицы
	if _, err := db.Exec(`FLUSH TABLES WITH READ LOCK;`); err != nil {
		log.Fatal(err)
	}

	repoName := "mysql-snapshot"
	tagName := time.Now().Format("20060102-150405")

	// Сохраняем контейнер в новый образ == из diff'а файловой системы делаем новый слой
	commitOptions := types.ContainerCommitOptions{
		ContainerID:    container.ID,
		RepositoryName: repoName,
		Tag:            tagName,
		Comment:        "Snapshooter",
		Author:         "Snapshooter",
		Pause:          true,
	}
	response, err := cli.ContainerCommit(commitOptions)

	fmt.Printf("Created new image with ID %v\n", response.ID)
	if err != nil {
		log.Fatal(err)
	}

	// Разблокируем таблицы
	// TODO: [MySQL] 2016/02/04 12:15:20 packets.go:32: unexpected EOF
	if _, err := db.Exec(`UNLOCK TABLES;`); err != nil {
		log.Print(err)
	}

	fmt.Println("\nStart container by:")
	fmt.Printf("\tdocker run -d -P -e 'affinity:container==slave-mysql' %s:%s\n", repoName, tagName)
}
