package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"os"
	"sync"
	"time"

	docker "github.com/fsouza/go-dockerclient"
)

// Upstreams хранит в себе список хостов, куда мы будем
// проксировать запрос
type Upstreams struct {
	sync.Mutex
	hosts map[string]string
}

// Add добавляет новых хост в список апстримов
func (u *Upstreams) Add(container, host, port string) {
	u.Lock()
	defer u.Unlock()

	if u.hosts == nil {
		u.hosts = make(map[string]string)
	}

	u.hosts[container] = fmt.Sprintf("%v:%v", host, port)
}

// Remove удаляет контейнер из списка
func (u *Upstreams) Remove(container string) {
	u.Lock()
	defer u.Unlock()

	if _, ok := u.hosts[container]; ok {
		delete(u.hosts, container)
	}
}

// Get делает типа балансировку :)
func (u *Upstreams) Get() string {
	u.Lock()
	defer u.Unlock()

	// Берёт случайный апстрим
	r := rand.New(rand.NewSource(time.Now().Unix()))
	idx := r.Intn(len(u.hosts))

	var host string
	var i int
	for _, upstream := range u.hosts {
		if i == idx {
			host = upstream
			break
		}
		i++
	}
	return host
}

// Наши upstream'ы, адреса серверов, куда будет проксировать запрос
var (
	upstreams Upstreams
	client    *docker.Client
)

func main() {
	// Определяем порт, на котором будем слушать
	port := "80"
	if os.Getenv("PORT") != "" {
		port = os.Getenv("PORT")
	}

	// Создаем клиент докера из переменных окружения
	var err error
	client, err = docker.NewClientFromEnv()
	if err != nil {
		log.Fatalf("Failed to create docker-client: %v", err)
	}

	// Добавляем существующие контейнеры
	containers, err := client.ListContainers(
		docker.ListContainersOptions{All: true},
	)
	if err != nil {
		log.Fatalf("Failed to get list of containers: %v", err)
	}
	for _, container := range containers {
		addContainer(container.ID)
	}

	// Создаем канал для событий и подписываемся на них
	eventChan := make(chan *docker.APIEvents, 100)
	if err := client.AddEventListener(eventChan); err != nil {
		log.Fatalf("Failed to subscibe to docker events: %v", err)
	}
	go dockerEventListener(eventChan)

	// Создаем и настраиваем прокси
	proxy := &httputil.ReverseProxy{Director: proxyDirector}

	// Простой хендлер, который показывает хост и текущее время
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t := time.Now()
		proxy.ServeHTTP(w, r)
		// Ну и access.log для наглядности
		log.Printf("%s %q %v\n", r.Method, r.URL.String(), time.Since(t))
	})
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// proxyDirector управляет логикой проксирования
func proxyDirector(req *http.Request) {
	host := upstreams.Get()

	req.URL.Scheme = "http"
	req.URL.Host = host
}

// dockerEventListener читает из канала события от докера и реагирует на них
func dockerEventListener(events chan *docker.APIEvents) {
	for {
		select {
		case event, ok := <-events:
			// Вдруг канал уже закрыт и события кончились
			if !ok {
				log.Printf("Docker daemon connection interrupted")
				break
			}

			// Если был запущен контейнер, проверяем что у него есть 22 порт и добавляем
			if event.Status == "start" {
				addContainer(event.ID)
			}

			// Если контейнер был удалён/убит, убераем его
			if event.Status == "stop" || event.Status == "die" {
				log.Printf("Container %s was removed", event.ID[:12])
				upstreams.Remove(event.ID)
			}

		// Пингуем клиент, чтобы про нас не забывали
		case <-time.After(10 * time.Second):
			if err := client.Ping(); err != nil {
				log.Printf("Unable to ping docker daemon: %s", err)
			}
		}
	}
}

// containerCreated обрабатывает событие создания контейнера
func addContainer(containerID string) {
	log.Printf("Container %s was started", containerID[:12])

	container, _ := client.InspectContainer(containerID)
	for port, mapping := range container.NetworkSettings.Ports {
		if port == "80/tcp" {
			log.Printf("Added %v: %s:%v\n",
				container.ID[:12],
				mapping[0].HostIP,
				mapping[0].HostPort,
			)

			upstreams.Add(container.ID, mapping[0].HostIP, mapping[0].HostPort)
		}
	}
}
