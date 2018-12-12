package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
)

//GitlabRepository represents repository information from the webhook
type GitlabRepository struct {
	Name        string
	URL         string
	Description string
	Home        string
}

//Commit represents commit information from the webhook
type Commit struct {
	ID        string
	Message   string
	Timestamp string
	URL       string
	Author    Author
}

//Author represents author information from the webhook
type Author struct {
	Name  string
	Email string
}

//Webhook represents push information from the webhook
type Webhook struct {
	Before            string
	After             string
	Ref               string
	Username          string
	UserID            int
	ProjectID         int
	Repository        GitlabRepository
	Commits           []Commit
	TotalCommitsCount int
}

//ConfigRepository represents a repository from the config file
type ConfigRepository struct {
	Name     string
	Commands []string
}

//Config represents the config file
type Config struct {
	Logfile      string
	Address      string
	Port         int64
	Repositories []ConfigRepository
}

func PanicIf(err error, what ...string) {
	if err != nil {
		if len(what) == 0 {
			panic(err)
		}

		panic(errors.New(err.Error() + what[0]))
	}
}

var config Config
var configFile string

func main() {
	args := os.Args

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGHUP)

	go func() {
		<-sigc
		var err error
		config, err = loadConfig(configFile)
		if err != nil {
			log.Fatalf("Failed to read config: %s", err)
		}
		log.Println("config reloaded")
	}()

	//if we have a "real" argument we take this as conf path to the config file
	if len(args) > 1 {
		configFile = args[1]
	} else {
		configFile = "config.json"
	}

	//load config
	config, err := loadConfig(configFile)
	if err != nil {
		log.Fatalf("Failed to read config: %s", err)
	}

	//open log file
	writer, err := os.OpenFile(config.Logfile, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0666)
	if err != nil {
		log.Printf("Failed to open log file: %s", err)
		os.Exit(1)
	}

	//close logfile on exit
	defer func() {
		writer.Close()
	}()

	//setting logging output
	log.SetOutput(writer)

	//setting handler
	http.HandleFunc("/", hookHandler)

	address := config.Address + ":" + strconv.FormatInt(config.Port, 10)

	log.Println("Listening on " + address)

	//starting server
	err = http.ListenAndServe(address, nil)
	if err != nil {
		log.Println(err)
	}
}

func loadConfig(configFile string) (Config, error) {
	file, err := os.Open(configFile)
	if err != nil {
		return Config{}, err
	}
	defer file.Close()

	buffer := make([]byte, 1024)
	count := 0

	count, err = file.Read(buffer)
	if err != nil {
		return Config{}, err
	}

	err = json.Unmarshal(buffer[:count], &config)
	if err != nil {
		return Config{}, err
	}

	return config, nil
}

func hookHandler(w http.ResponseWriter, r *http.Request) {
	var hook Webhook

	//read request body
	var data, err = ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Failed to read request: %s", err)
		return
	}

	//unmarshal request body
	err = json.Unmarshal(data, &hook)
	if err != nil {
		log.Printf("Failed to parse request: %s", err)
		return
	}

	//find matching config for repository name
	for _, repo := range config.Repositories {
		if repo.Name != hook.Repository.Name {
			continue
		}

		//execute commands for repository
		for _, cmd := range repo.Commands {
			var command = exec.Command(cmd)
			out, err := command.Output()
			if err != nil {
				log.Printf("Failed to execute command: %s", err)
				continue
			}
			log.Println("Executed: " + cmd)
			log.Println("Output: " + string(out))
		}
	}
}
