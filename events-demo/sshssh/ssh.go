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

// SSHServer реализует ssh-сервер, который дублирует запросы на список хостов
type SSHServer struct {
	sync.Mutex
	hosts  map[string]string
	config *ssh.ServerConfig
}

// NewSSHServer создает новый SSHServer :)
func NewSSHServer() (*SSHServer, error) {
	config := &ssh.ServerConfig{
		NoClientAuth: true, // пускаем всех, это же демка
		// Если NoClientAuth поставить в false, то можно сделать проверку паролей или ключей
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
		return nil, fmt.Errorf("failed to read id_rsa: %v", err)
	}
	private, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}
	config.AddHostKey(private)

	return &SSHServer{
		hosts:  make(map[string]string),
		config: config,
	}, nil
}

func (s *SSHServer) AddHost(container, host string) {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()

	s.hosts[container] = host
}

func (s *SSHServer) RemoveContainer(container string) {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()

	if _, ok := s.hosts[container]; ok {
		delete(s.hosts, container)
	}
}

func (s *SSHServer) ListenAndServe(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %v: %v", addr, err)
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			return fmt.Errorf("failed to accept connection: %v", err)
		}

		go s.connectionHandler(conn)
	}
}

func (s *SSHServer) connectionHandler(conn net.Conn) {
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
			log.Printf("Failed to accept: %v", err)
			return
		}
		for request := range requests {
			switch request.Type {
			case "exec":
				if err := s.execHandler(channel, request); err != nil {
					log.Printf("Failed to handle 'exec': %v", err)
					return
				}

			case "shell":
				if err := s.shellHandler(channel); err != nil {
					log.Printf("Failed to handle 'shell': %v", err)
					return
				}

			}
		}
	}
}

func (s *SSHServer) execHandler(channel ssh.Channel, request *ssh.Request) error {
	defer channel.Close()

	var payload = struct {
		Value string
	}{}
	if err := ssh.Unmarshal(request.Payload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %v", err)
	}

	result, status, err := s.runCmd(payload.Value)
	if err != nil {
		return fmt.Errorf("failed to run command: %v", err)
	}
	if err := sendCmdResult(channel, result, status); err != nil {
		return fmt.Errorf("failed to send result: %v", err)
	}
	if err := request.Reply(true, nil); err != nil {
		return fmt.Errorf("failed to send reply: %v", err)
	}
	return nil
}

func (s *SSHServer) shellHandler(channel ssh.Channel) error {
	defer channel.Close()

	term := terminal.NewTerminal(channel, "# ")
	for {
		line, err := term.ReadLine()
		if err != nil {
			break
		}
		if line == "" {
			return nil
		}

		result, status, err := s.runCmd(line)
		if err != nil {
			return fmt.Errorf("failed to run command: %v", err)
		}
		if err := sendCmdResult(channel, result, status); err != nil {
			return fmt.Errorf("failed to send result: %v", err)
		}
	}
	return nil
}

func (s *SSHServer) runCmd(cmd string) ([]byte, uint32, error) {
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

	wg.Wait()
	for idx := 1; idx <= len(s.hosts); idx++ {
		result.WriteString(<-resultsChan)
	}
	return result.Bytes(), 0, nil
}

// exec выполняет команду по ssh, в нашем случае с фиксированным юзером
func exec(cmd, host string, env map[string]string) ([]byte, error) {
	// Это демка, так что логин пароль захардкожены (используем образ ubuntu-upstart)
	conn, err := ssh.Dial("tcp", host, &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.Password("docker.io"),
		},
	})
	if err != nil {
		return []byte{}, fmt.Errorf("failer to connect to host: %v", err)
	}

	session, err := conn.NewSession()
	if err != nil {
		return []byte{}, fmt.Errorf("failer to start new ssh session: %v", err)
	}

	defer func() {
		conn.Close()
		session.Close()
	}()

	for key, value := range env {
		if err := session.Setenv(key, value); err != nil {
			return []byte{}, fmt.Errorf("failer to set env var: %v", err)
		}
	}
	return session.CombinedOutput(cmd)
}

func sendCmdResult(channel ssh.Channel, result []byte, statusCode uint32) error {
	if _, err := channel.Write(result); err != nil {
		return fmt.Errorf("failed to write to ssh-channel: %v", err)
	}
	status := struct {
		Status uint32
	}{
		statusCode,
	}
	_, err := channel.SendRequest("exit-status", false, ssh.Marshal(&status))
	if err != nil {
		return fmt.Errorf("failed to SendRequest: %v", err)
	}
	return nil
}
