package main

import (
	"log"
	"os"
	"time"

	"github.com/hibiken/asynq"
	"github.com/sumeshmurali/dirwatcher/internal"
)

// runPeriodicalTaskManager creates and runs the periodical task manager
func runPeriodicalTaskManager() {
	r, err := internal.NewRepository()
	if err != nil {
		log.Fatalf("taskmgr.runPeriodicalTaskManager: Unable to initialize new repository due to %v", err)
	}
	defer r.Close()

	p := &internal.DynamicTaskConfigProvider{Repo: r}
	i, err := time.ParseDuration(os.Getenv("TASKMGR_SYNC_INTERVAL"))
	if err != nil {
		log.Fatalf("taskmgr.runPeriodicalTaskManager: Unable to parse sync interval %v due to %v", os.Getenv("TASKMGR_SYNC_INTERVAL"), err)
	}
	mgr, err := asynq.NewPeriodicTaskManager(
		asynq.PeriodicTaskManagerOpts{
			RedisConnOpt:               asynq.RedisClientOpt{Addr: os.Getenv("REDIS_ADDR")},
			PeriodicTaskConfigProvider: p,
			SyncInterval:               i,
		})
	if err != nil {
		log.Fatal(err)
	}
	if err = mgr.Run(); err != nil {
		log.Fatal(err)
	}
}

// runTaskWorker runs a worker task to consume tasks
func runTaskWorker() {

	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: os.Getenv("REDIS_ADDR")},
		asynq.Config{
			// Specify how many concurrent workers to use
			Concurrency: 1,
		},
	)

	mux := asynq.NewServeMux()
	mux.HandleFunc(internal.DirWatcherTask, internal.HandleDirWatcherTask)

	if err := srv.Run(mux); err != nil {
		log.Fatalf("taskmgr.runTaskWorker: Could not start the worker server due to %v", err)
	}
}

func main() {
	internal.LoadEnv()
	// send the periodical task manager to background
	go runPeriodicalTaskManager()
	// TODO improve the below code by seperating the task manager and worker
	runTaskWorker()
}
