package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	ps "github.com/mitchellh/go-ps"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v2"
)

const (
	OutputSuccess = iota
	OutputFailure
)

type Task struct {
	TaskName   string `yaml:"task_name"`
	Schedule   string
	Command    string
	SaveOutput string `yaml:"save_output"`
	Enabled    bool
}

type Config struct {
	TaskDirectory   string
	OutputDirectory string
	Shell           string
	HealthCheckPort int
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
		log.Println(err)
	}

	for _, f := range files {
		filename, _ := filepath.Abs(f)
		yamlFile, err := ioutil.ReadFile(filename)

		if err != nil {
			log.Println(err)
			continue
		}

		var task Task
		task.Enabled = true
		task.SaveOutput = "on-failure,on-success"
		err = yaml.Unmarshal(yamlFile, &task)
		if err != nil {
			promYAMLParseErrors.WithLabelValues(path.Base(f)).Inc()
			log.Println(err)
			continue
		}

		if len(task.Command) == 0 || len(task.Schedule) == 0 || len(task.TaskName) == 0 {
			promYAMLParseErrors.WithLabelValues(path.Base(f)).Inc()
			log.Println(fmt.Sprintf("Missing task attributes in %s", path.Base(f)))
			continue
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

func writeOutput(task Task, content, filename, outputDirectory string) {
	taskDirectory := filepath.Join(outputDirectory, task.TaskName)
	if _, err := os.Stat(taskDirectory); os.IsNotExist(err) {
		err = os.MkdirAll(taskDirectory, os.ModePerm)
		if err != nil {
			log.Println("error:", err)
			return
		}
	}
	data := []byte(content)
	err := ioutil.WriteFile(filepath.Join(taskDirectory, filename), data, 0644)
	if err != nil {
		log.Println("error:", err)
	}
}

func notifyTaskStarted(task Task, outputDirectory string) {
	ts := time.Now().String()
	writeOutput(task, ts, "last-start", outputDirectory)
}

func notifyTaskSucceeded(task Task, outputDirectory string) {
	ts := time.Now().String()
	writeOutput(task, ts, "last-success", outputDirectory)
}

func notifyTaskFailed(task Task, outputDirectory string) {
	ts := time.Now().String()
	writeOutput(task, ts, "last-failure", outputDirectory)
}

func writeTaskOutput(task Task, outputType int, output, outputDirectory string) {
	if outputType == OutputSuccess && !strings.Contains(task.SaveOutput, "on-success") {
		return
	}
	if outputType == OutputFailure && !strings.Contains(task.SaveOutput, "on-failure") {
		return
	}

	tsUnix := time.Now().Unix()
	ts := time.Now().String()
	content := fmt.Sprintf("%s\n%s", ts, output)
	filename := fmt.Sprintf(outputFilePattern[outputType], tsUnix)
	writeOutput(task, content, filename, outputDirectory)
}

var (
	config              Config
	outputFilePattern   map[int]string
	promYAMLParseErrors = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "scheduler_yaml_parse_failure",
		Help: "Malformed YAML file",
	}, []string{"file_name"})
	promJobParseErrors = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "scheduler_job_parse_failure",
		Help: "Job parse error",
	}, []string{"job_name"})
)

func init() {
	flag.StringVar(&config.TaskDirectory, "c", "./example-tasks", "says where the tasks directory with YAML files is located")
	flag.StringVar(&config.OutputDirectory, "o", "/var/spool/bob-the-scheduler", "where to save the jobs output")
	flag.StringVar(&config.Shell, "sh", "/bin/sh", "shell path")
	flag.IntVar(&config.HealthCheckPort, "p", 8000, "healthcheck port (e.g. http://localhost:8000/healhtz)")
	flag.BoolVar(&config.Verbose, "v", false, "increase verbosity")
	flag.Parse()

	outputFilePattern = make(map[int]string)
	outputFilePattern[OutputSuccess] = "stdout-%d.succeeded"
	outputFilePattern[OutputFailure] = "stdout-%d.failed"
}

func main() {
	wg := &sync.WaitGroup{}
	wg.Add(1)

	c := cron.New()

	healthCheckHandler := func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintf(w, "tasks: %d\n", len(c.Entries()))
		processes, err := ps.Processes()
		if err != nil {
			fmt.Fprintf(w, "processes: N/A\n")
			log.Print(err)
		} else {
			fmt.Fprintf(w, "processes: %d\n", len(processes))
		}
	}

	http.HandleFunc("/healhtz", healthCheckHandler)
	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(fmt.Sprintf(":%d", config.HealthCheckPort), nil)

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

				notifyTaskStarted(currentTask, config.OutputDirectory)

				out, err := exec.Command(config.Shell, "-c", currentTask.Command).Output()
				if err != nil {
					notifyTaskFailed(currentTask, config.OutputDirectory)
					writeTaskOutput(currentTask, OutputFailure, err.Error(), config.OutputDirectory)
					log.Println("error:", err)
				} else {
					notifyTaskSucceeded(currentTask, config.OutputDirectory)
					writeTaskOutput(currentTask, OutputSuccess, string(out), config.OutputDirectory)
					log.Printf("[%s] output is:\n%s\n", currentTask.TaskName, out)
				}

				if config.Verbose {
					log.Printf("Ending task %s", currentTask.TaskName)
				}
			}

			_, err := c.AddFunc(task.Schedule, callback)

			if err != nil {
				promJobParseErrors.WithLabelValues(task.TaskName).Inc()
				log.Println("error:", err)
			}
		}
	}

	processTaskDirectory()
	c.Start()
	watchDirectory(config, processTaskDirectory)

	wg.Wait()
}
