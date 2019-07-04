package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"golang.org/x/crypto/ssh"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

type Task struct {
	id       int
	hostname string
	cmd      string
}

type Result struct {
	task   Task
	stdout string
}

type Option struct {
	hostname string
	port     int
	username string
	password string
	command  string
	workers  int
	timeout  int
}

const (
	ErrConRefused = "Error: Connection Refused!"
)

var tasks = make(chan Task, 100)
var results = make(chan Result, 100)

func runCmd(cmd, hostname string) string {
	hostname = strings.TrimSpace(hostname)

	if !strings.Contains(hostname, ":") {
		hostname = hostname + ":22"
	}
	client, session, err := getConnection(username, password, hostname)
	if err != nil {
		//panic(err)
		return ErrConRefused
	}
	defer session.Close()
	defer client.Close()
	var stdout bytes.Buffer
	session.Stdout = &stdout
	session.Run(cmd)
	return stdout.String()
}

func worker(group *sync.WaitGroup) {
	for task := range tasks {
		output := Result{task, runCmd(task.cmd, task.hostname)}
		results <- output
	}
	group.Done()
}

func createWorkerPool(workers int) {
	var group sync.WaitGroup
	for i := 0; i < workers; i++ {
		group.Add(1)
		go worker(&group)
	}
	group.Wait()
	close(results)
}

func distribute(numTasks int, hosts []string, cmd string) {
	for i := 0; i < numTasks; i++ {
		task := Task{i, hosts[i], cmd}
		tasks <- task
	}
	close(tasks)
}

type Response struct {
	TaskId   int    `json:"taskId"`
	Hostname string `json:"hostname"`
	StdOut   string `json:"stdout"`
}

func result(done chan bool) {
	for result := range results {
		response := &Response{
			TaskId:   result.task.id,
			Hostname: result.task.hostname,
			StdOut:   result.stdout,
		}
		responseBytes, _ := json.Marshal(response)
		fmt.Println(string(responseBytes))
	}
	done <- true
}

func getConnection(user, password, host string) (*ssh.Client, *ssh.Session, error) {
	sshConfig := &ssh.ClientConfig{
		User:    user,
		Auth:    []ssh.AuthMethod{ssh.Password(password)},
		Timeout: 5 * time.Second,
	}
	sshConfig.HostKeyCallback = ssh.InsecureIgnoreHostKey()

	client, err := ssh.Dial("tcp", host, sshConfig)
	if err != nil {
		return nil, nil, err
	}

	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, nil, err
	}

	return client, session, nil
}

// Load hostnames or IP Addresses from the configuration file
func loadHosts(path string) ([]string, error) {
	var lines []string
	if !isFile(path) {
		return lines, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}

func isFile(path string) bool {
	return !isDir(path)
}

var hostFile string
var hosts string
var username string
var password string
var port int
var command string
var workers int
var timeout int

func main() {
	log.Println("Launching sshexec...")
	startTime := time.Now()
	flag.StringVar(&hosts, "host", "", "The hostname or IP address of the remote host")
	flag.IntVar(&port, "port", 22, "specify ssh port to use.  defaults to 22.")
	flag.StringVar(&username, "username", "", "username on the remote host")
	flag.StringVar(&password, "password", "", "password on the remote host")
	flag.StringVar(&command, "command", "", "command to run on the remote host")
	flag.IntVar(&workers, "workers", 100, "specify the number of workers.")
	flag.IntVar(&timeout, "timeout", 100, "specify the connection timeout.")

	flag.Usage = func() {
		fmt.Printf("Usage of %s:\n", os.Args[0])
		fmt.Printf("sshexec\t-hosts hosts \\\n" +
			"\t-username appuser \\\n" +
			"\t-password 'password' \\\n" +
			"\t-command 'jps\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	options := Option{
		hostname: hosts,
		port:     port,
		username: username,
		password: password,
		command:  command,
		workers:  workers,
		timeout:  timeout,
	}

	fmt.Printf("hosts: %s\n", options.hostname)
	fmt.Printf("port: %d\n", options.port)
	fmt.Printf("username: %s\n", options.username)
	fmt.Printf("password: %s\n", options.password)
	fmt.Printf("command: %s\n", options.command)

	hostname, _ := loadHosts("hosts")
	fmt.Printf("hostnames: %v\n", hostname)

	numTasks := len(hostname)
	go distribute(numTasks, hostname, command)
	done := make(chan bool)
	go result(done)
	createWorkerPool(workers)
	<-done
	endTime := time.Now()
	diff := endTime.Sub(startTime)
	log.Println("total time taken ", diff.Seconds(), "seconds")
}
