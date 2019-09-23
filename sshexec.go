package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/sarathkumarsivan/sshexec/ioutil"
	"golang.org/x/crypto/ssh"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

type Task struct {
	id   int
	host string
	port int
	user string
	pass string
	cmd  string
}

type Result struct {
	task   Task
	stdout string
}

type Option struct {
	host    string
	user    string
	pass    string
	cmd     string
	workers int
	timeout int
}

const (
	ErrConRefused = "Error: Connection Refused!"
)

const DefaultSSHPort int = 22
const DefaultWorkers int = 100
const DefaultTimeout int = 60

var tasks = make(chan Task, 100)
var results = make(chan Result, 100)

func runCmd(host string, port int, user, pass, cmd string) string {
	host = strings.TrimSpace(host)

	if !strings.Contains(host, ":") {
		host = host + ":22"
	}
	client, session, err := connect(user, port, pass, host)
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
		output := Result{task, runCmd(task.host, task.port, task.user, task.pass, task.cmd)}
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
		task := Task{i, hosts[i], DefaultSSHPort, "", "", cmd}
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
			Hostname: result.task.host,
			StdOut:   result.stdout,
		}
		responseBytes, _ := json.Marshal(response)
		fmt.Println(string(responseBytes))
	}
	done <- true
}

func connect(host string, port int, user, pass string) (*ssh.Client, *ssh.Session, error) {
	conf := &ssh.ClientConfig{
		User:    user,
		Auth:    []ssh.AuthMethod{ssh.Password(pass)},
		Timeout: 5 * time.Second,
	}

	conf.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), conf)

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

// Read lines from the specified file.
func ReadLines(path string) ([]string, error) {
	var lines []string

	if !ioutil.IsFile(path) {
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

func main() {
	log.Println("Launching sshexec...")
	startTime := time.Now()

	var host = flag.String("host", "", "Hostname or IP Address of the remote server")
	var port = flag.Int("port", DefaultSSHPort, "Port of the remote server")
	var user = flag.String("user", "", "User who runs the ssh task")
	var pass = flag.String("pass", "", "Plain text password to run ssh task")
	var cmd = flag.String("cmd", "", "Plain text password to run ssh task")
	var workers = flag.Int("workers", DefaultWorkers, "Specify the number of concurrent tasks")
	var timeout = flag.Int("timeout", 100, "Specify SSH connection timeout")

	flag.Usage = func() {
		fmt.Printf("Usage of %s:\n", os.Args[0])
		fmt.Printf("sshexec\t \n" +
			"\t-host 127.0.0.1 \n" +
			"\t-port 22 \n" +
			"\t-user appuser \n" +
			"\t-pass password \n" +
			"\t-cmd jps \n" +
			"\t-workers 100 \n" +
			"\t-timeout 50 \n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *host == "" || *user == "" || *pass == "" || *cmd == "" {
		flag.Usage()
		os.Exit(1)
	}

	fmt.Printf("hosts: %s\n", host)
	fmt.Printf("port: %d\n", port)
	fmt.Printf("user: %s\n", user)
	fmt.Printf("pass: %s\n", pass)
	fmt.Printf("cmd: %s\n", cmd)
	fmt.Printf("workers: %s\n", workers)
	fmt.Printf("timeout: %s\n", timeout)

	var hosts, _ = ReadLines(*host)
	fmt.Printf("hostnames: %v\n", host)

	numTasks := len(hosts)
	go distribute(numTasks, hosts, *cmd)
	done := make(chan bool)
	go result(done)
	createWorkerPool(*workers)
	<-done
	endTime := time.Now()
	diff := endTime.Sub(startTime)
	log.Println("Task completed!")
	log.Println("Total time taken ", diff.Seconds(), "seconds")
}
