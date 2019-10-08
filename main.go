package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v2"
)

type Task struct {
	TaskName string `yaml:"task_name"`
	Schedule string
	Command  string
	Enabled  bool
}

type Config struct {
	TaskDirectory   string
	OutputDirectory string
	Shell           string
	HealthCheck     bool
	Verbose         bool
}

func getTasks(config Config) []Task {
	var tasks []Task
	var files []string

	err := filepath.Walk(config.TaskDirectory, func(path string, info os.FileInfo, err error) error {
		if filepath.Ext(path) == ".yml" || filepath.Ext(path) == ".yaml" {
			files = append(files, path)
		}
		return nil
	})

	if err != nil {
		log.Fatal(err)
	}

	for _, f := range files {
		filename, _ := filepath.Abs(f)
		yamlFile, err := ioutil.ReadFile(filename)

		if err != nil {
			log.Fatal(err)
			continue
		}

		var task Task
		task.Enabled = true
		err = yaml.Unmarshal(yamlFile, &task)
		if err != nil {
			continue
			log.Fatal(err)
		}

		tasks = append(tasks, task)
	}

	return tasks
}

func watchDirectory(config Config, callback func()) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Println("error:", err)
	}
	defer watcher.Close()

	done := make(chan bool)

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					if config.Verbose {
						log.Println("modified file:", event.Name)
					}
					callback()
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("error:", err)
			}
		}
	}()

	if err := watcher.Add(config.TaskDirectory); err != nil {
		log.Println("error:", err)
	}

	<-done
}

var config Config

func init() {
	flag.StringVar(&config.TaskDirectory, "c", "./example-tasks", "says where the tasks directory with YAML files is located")
	flag.StringVar(&config.OutputDirectory, "o", "/var/spool/bob-the-scheduler", "where to save the jobs output")
	flag.StringVar(&config.Shell, "sh", "/bin/sh", "shell path")
	flag.BoolVar(&config.HealthCheck, "p", false, "provides healthcheck endpoint on http://localhost:8000/healhtz")
	flag.BoolVar(&config.Verbose, "v", false, "increase verbosity")
	flag.Parse()
}

func main() {
	wg := &sync.WaitGroup{}
	wg.Add(1)

	c := cron.New()

	healthCheckHandler := func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintf(w, "tasks: %d", len(c.Entries()))
	}

	if config.HealthCheck {
		http.HandleFunc("/healhtz", healthCheckHandler)
		go http.ListenAndServe(":8000", nil)
	}

	processTaskDirectory := func() {
		for _, entry := range c.Entries() {
			if config.Verbose {
				log.Printf("Removing task #%d", entry.ID)
			}
			c.Remove(entry.ID)
		}

		for _, task := range getTasks(config) {
			if !task.Enabled {
				if config.Verbose {
					log.Printf("Skipping task %s - task not enabled", task.TaskName)
				}
				continue
			}

			if config.Verbose {
				log.Printf("Adding task %s", task.TaskName)
			}

			currentTask := task
			callback := func() {
				if config.Verbose {
					log.Printf("Starting task %s", currentTask.TaskName)
				}

				out, err := exec.Command(config.Shell, "-c", currentTask.Command).Output()

				if err != nil {
					log.Println("error:", err)
				}
				fmt.Printf("[%s] output is:\n%s\n", currentTask.TaskName, out)

				if config.Verbose {
					log.Printf("Ending task %s", currentTask.TaskName)
				}
			}

			_, err := c.AddFunc(task.Schedule, callback)

			if err != nil {
				log.Println("error:", err)
			}
		}
	}

	processTaskDirectory()
	c.Start()
	watchDirectory(config, processTaskDirectory)

	wg.Wait()
}
