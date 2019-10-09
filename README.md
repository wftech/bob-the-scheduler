# Bob the Scheduler 

Bob the Scheduler is program used to schedule tasks. It plays nicely with 
Docker (or any other containers) and integrates with  file-based 
configurations.

Bob's mission is to provide application developers a convenient way to schedule repeating jobs. It is intended to be run as 
PID 1 in container and spawn jobs.

If you need more advanced tasks like running tasks in other container, 
check [Ofelia][ofelia] or [K8S cronjobs][k8s-cronjob].

## How to use

Create directory `/etc/bob-the-scheduler/tasks` with jobs described as YAML files
The job description should be saved with `.yml` or `.yaml` extension.

```!yaml
# task-01.yaml
task_name:  task-01
schedule: "@daily"
command: touch /var/data/timestamp
enabled: no

save_output: on-failure,on-success
```    

```!yaml
# generate-images.yml
task_name: update-images
schedule: "CRON_TZ=Europe/Bratislava 12 01-07 * * *"
command: php /usr/local/www/cronjobs/update-images.php
```

## How to start

`bob-the-scheduler -c /etc/bob-the-scheduler -o /var/spool/bob-the-scheduler/ -p 8080 -v`

Bob will periodically check new files in the config directory a and schedule its job.

## Used command line parameters

* `-c` says where the `tasks` directory with YAML files is located, 
* `-o` where to save the jobs output.
* `-p` provides healthcheck endpoint on http://localhost:8000/healhtz
* `-v` increase verbosity

The scheduling is done in UTC, but you can use [CRON_TZ][cron-tz] syntax
to schedule in any other zone.

All the jobs are started with UID/GID of the calling user.

## Job run

* When the job is going to be started, Bob creates `/var/spool/bob-the-scheduler/task-name/` directory.
* The job will get timestamp.
* Bob will create/overwrite file `/task-name/last-start` containing timestamp
* It will create `/task-name/stdout-timestamp.running` file and redirects job command stdout there.
* Jobs stderr is not going to be redirected
* After successfull run, file `/task-name/last-success` is created/overwritten (containing timestamp)
* After successfull run  `stdout` file is moved to `stdout-timestamp.succeeded`
* After failed run, file `/task-name/last-failure` is created/overwritten (containing timestamp)
* After failed run  `stdout` file is moved to `stdout-timestamp.failed`


# Job parameters

* `task_name`: ob name to use in file names. Defaults to job file name without 
* `schedule`: (required). Job schedule according to [robfig's cron][go-cron]. Re
* `command`: (required). Command to run
* `enabled`: should the job be run? Defaults to True.
* `save_output`: Should we keep the job output on success or failure? Default `on-success,on-failure`


# What Bob does not do?

* Mail reports on failures.
* Enforce resource constraints (CPU/memory).
* Report tasks' state through HTTP API.
* Change UID/GID on execution.
* Cleanup its files.

Patches are welcome. 


[ofelia]: https://github.com/mcuadros/ofelia
[k8s-cronjob]: https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/
[go-cron]: https://godoc.org/github.com/robfig/cron
[cron-tz]: https://godoc.org/github.com/robfig/cron#hdr-Time_zones
