package main

/*

Для вдохновления использовались
https://github.com/crosbymichael/slex
https://github.com/gliderlabs/sshfront

*/

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/terminal"
)

type sshServer struct {
	sync.Mutex
	hosts  map[string]string
	config *ssh.ServerConfig
}

func NewSSHServer() (*sshServer, error) {
	config := &ssh.ServerConfig{
		NoClientAuth: true, // пускаем всех, это же демка
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			log.Printf("Welcome user '%s' with password '%s'", conn.User(), string(password))
			return &ssh.Permissions{
				Extensions: map[string]string{
					"user": conn.User(),
				},
			}, nil
		},
	}

	privateBytes, err := ioutil.ReadFile("id_rsa")
	if err != nil {
		return nil, err
	}
	private, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil {
		return nil, err
	}
	config.AddHostKey(private)

	return &sshServer{
		hosts:  make(map[string]string),
		config: config,
	}, nil
}

func (s *sshServer) ListenAndServe(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}

		go func() {
			defer conn.Close()
			_, chans, reqs, err := ssh.NewServerConn(conn, s.config)
			if err != nil {
				log.Printf("ERROR: %v", err)
				return
			}
			go ssh.DiscardRequests(reqs)

			for newChannel := range chans {
				if newChannel.ChannelType() != "session" {
					newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
					continue
				}
				channel, requests, err := newChannel.Accept()
				if err != nil {
					log.Printf("ERROR: %v", err)
					return
				}
				go s.handleChannel(channel, requests)
			}
		}()
	}
}

func (s *sshServer) handleChannel(channel ssh.Channel, requests <-chan *ssh.Request) {
	for req := range requests {
		switch req.Type {
		case "exec":
			var payload = struct {
				Value string
			}{}
			ssh.Unmarshal(req.Payload, &payload)

			result, status, err := s.runCmd(payload.Value)
			if err != nil {
				log.Printf("ERROR: %v", err)
				channel.Close()
				continue
			}
			sendCmdResult(channel, result, status)
			req.Reply(true, nil)
			channel.Close()

		case "shell":
			go func() {
				term := terminal.NewTerminal(channel, "# ")
				defer channel.Close()
				for {
					line, err := term.ReadLine()
					if err != nil {
						break
					}
					if line == "" {
						continue
					}

					result, status, err := s.runCmd(line)
					if err != nil {
						log.Printf("ERROR: %v", err)
						channel.Close()
						continue
					}
					sendCmdResult(channel, result, status)
				}
			}()
		}
	}
}

func (s *sshServer) AddHost(container, host string) {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()

	s.hosts[container] = host
}

func (s *sshServer) RemoveContainer(container string) {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()

	if _, ok := s.hosts[container]; ok {
		delete(s.hosts, container)
	}
}

func (s *sshServer) runCmd(cmd string) ([]byte, uint32, error) {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()

	resultsChan := make(chan string, len(s.hosts))

	var wg sync.WaitGroup
	var result bytes.Buffer
	wg.Add(len(s.hosts))

	// Выполняем команду на всех хостах и ждём выполнения
	for _, host := range s.hosts {
		go func(cmd, host string) {
			defer wg.Done()

			result, err := exec(cmd, host, nil)
			if err != nil {
				log.Println(err)
			}
			resultsChan <- fmt.Sprintf("[%s] %s", host, result)
			log.Printf("Result of '%s:%s': %v", host, cmd, string(result))
		}(cmd, host)
	}

	// go func() {
	// 	for buf := range resultsChan {
	// 		result.WriteString(buf)
	// 	}
	// }()

	wg.Wait()
	for idx := 1; idx <= len(s.hosts); idx++ {
		result.WriteString(<-resultsChan)
	}
	close(resultsChan)
	return result.Bytes(), 0, nil
}

// exec executes the given command on the given host
func exec(cmd, host string, env map[string]string) ([]byte, error) {
	// Это демка, так что логин пароль захардкожены (используем образ ubuntu-upstart)
	conn, err := ssh.Dial("tcp", host, &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.Password("docker.io"),
		},
	})
	if err != nil {
		return []byte{}, err
	}

	session, err := conn.NewSession()
	if err != nil {
		return []byte{}, err
	}

	defer func() {
		conn.Close()
		session.Close()
	}()

	for key, value := range env {
		if err := session.Setenv(key, value); err != nil {
			return []byte{}, err
		}
	}
	return session.CombinedOutput(cmd)
}

func sendCmdResult(channel ssh.Channel, result []byte, statusCode uint32) {
	channel.Write(result)
	status := struct {
		Status uint32
	}{
		statusCode,
	}
	channel.SendRequest("exit-status", false, ssh.Marshal(&status))
}
