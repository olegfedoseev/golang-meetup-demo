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
		log.Fatalf("Failed to create SSH-server: %v", err)
	}

	// Создаем клиент докера из переменных окружения
	client, err := docker.NewClientFromEnv()
	if err != nil {
		log.Fatalf("Failed to create docker-client: %v", err)
	}

	// Создаем канал для событий и подписываемся на них
	eventChan := make(chan *docker.APIEvents, 100)
	if err := client.AddEventListener(eventChan); err != nil {
		log.Fatalf("Failed to subscibe to docker events: %v", err)
	}
	go dockerEventListener(server, client, eventChan)
	getExistingContainers(server, client)

	log.Fatalln(server.ListenAndServe(":2222"))
}

// getExistingContainers получает от докера все запущенные контейнеры и добавляет
// в ssh-сервер те, у кого есть ssh
func getExistingContainers(server *SSHServer, client *docker.Client) {
	options := docker.ListContainersOptions{All: true}
	containers, err := client.ListContainers(options)
	if err != nil {
		log.Fatalf("Failed to get list of containers: %v", err)
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
}

// dockerEventListener читает из канала события от докера и реагирует на них
func dockerEventListener(server *SSHServer, client *docker.Client, events chan *docker.APIEvents) {
	for {
		select {
		case event, ok := <-events:
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
