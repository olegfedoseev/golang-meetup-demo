package main

import (
	"fmt"
	"log"
	"time"

	docker "github.com/fsouza/go-dockerclient"
)

// Запускать новые контейнеры с sshd можно примерно вот так:
// docker run -d -p 22 -e constraint:instance==backend ubuntu-upstart

func main() {
	// Создаем наш SSH-сервер
	server, err := NewSSHServer()
	if err != nil {
		log.Fatalln(err)
	}
	go server.ListenAndServe(":2222")

	// Создаем клиент докера из переменных окружения
	client, err := docker.NewClientFromEnv()
	if err != nil {
		log.Fatalln(err)
	}

	// Получаем список всех контейнеров
	options := docker.ListContainersOptions{All: true}
	containers, err := client.ListContainers(options)
	if err != nil {
		log.Fatalln(err)
	}

	for _, c := range containers {
		if len(c.Ports) == 0 {
			continue
		}
		for _, port := range c.Ports {
			// Если у контейнера есть 22 порт, то добавляем его в SSH-сервер
			if port.PrivatePort == 22 {
				log.Printf("Added %v: %s:%d\n", c.ID[:12], port.IP, port.PublicPort)
				server.AddHost(c.ID, fmt.Sprintf("%s:%d", port.IP, port.PublicPort))
			}
		}
	}

	// Создаем канал для событий и подписываемся на них
	eventChan := make(chan *docker.APIEvents, 100)
	if err := client.AddEventListener(eventChan); err != nil {
		log.Fatalln(err)
	}
	log.Println("Watching docker events")

	// Слушаем и реагируем на события
	for {
		select {
		case event, ok := <-eventChan:
			if !ok {
				log.Printf("Docker daemon connection interrupted")
				break
			}

			// Если был запущен контейнер, проверяем что у него есть 22 порт и добавляем
			if event.Status == "start" {
				log.Printf("Container %s was started", event.ID[:12])

				container, _ := client.InspectContainer(event.ID)
				if len(container.NetworkSettings.Ports) == 0 {
					continue
				}
				for port, mapping := range container.NetworkSettings.Ports {
					if port == "22/tcp" {
						log.Printf("Added %v: %s:%v\n", container.ID[:12], mapping[0].HostIP, mapping[0].HostPort)
						server.AddHost(container.ID, fmt.Sprintf("%s:%v", mapping[0].HostIP, mapping[0].HostPort))
					}
				}
			}

			// Если контейнер был удалён/убит, убераем его
			if event.Status == "stop" || event.Status == "die" {
				log.Printf("Container %s was removed", event.ID[:12])

				server.RemoveContainer(event.ID)
			}

		case <-time.After(10 * time.Second):
			if err := client.Ping(); err != nil {
				log.Printf("Unable to ping docker daemon: %s", err)
			}
		}
	}
}
