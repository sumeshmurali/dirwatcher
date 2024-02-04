// tasks handle the dynamic updation of the task configuration and execution of the tasks
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hibiken/asynq"
)

const DirWatcherTask = "dirwatcher:run"

// DynamicTaskConfigProvider fetches the tasks from the DB and provides to the task manager
type DynamicTaskConfigProvider struct {
	Repo Repository
}

// GetConfigs Gets the latest configurations from the repo
func (p *DynamicTaskConfigProvider) GetConfigs() ([]*asynq.PeriodicTaskConfig, error) {
	c, err := p.Repo.GetTaskConfig()
	if err != nil {
		return nil, err
	}

	payload, err := json.Marshal(c)
	if err != nil {
		log.Printf("tasks.GetConfigs: json.Marshal(%+v) failed with %v\n", c, err)
		return nil, err
	}
	config := asynq.PeriodicTaskConfig{
		Cronspec: fmt.Sprintf("@every %vs", c.Interval),
		Task:     asynq.NewTask(DirWatcherTask, payload),
	}
	var configs []*asynq.PeriodicTaskConfig
	configs = append(configs, &config)
	return configs, nil

}

// HandleDirWatcherTask handle DirWatcherTask tasks and inserts the results into repo
func HandleDirWatcherTask(ctx context.Context, t *asynq.Task) error {
	result := Result{}
	st := time.Now()
	result.StartTime = st.Unix()

	var c TaskConfig
	if err := json.Unmarshal(t.Payload(), &c); err != nil {
		return fmt.Errorf("tasks.HandleDirWatcherTask: json.Unmarshal(%v) failed with %v", t.Payload(), err)
	}
	result.MagicWord = c.MagicWord
	result.Status = "running"

	repo, err := NewRepository()
	if err != nil {
		return fmt.Errorf("tasks.HandleDirWatcherTask: repository.NewRepository failed with %v", err)
	}
	defer repo.Close()

	oid, err := repo.AddResult(result)
	if err != nil {
		log.Printf("tasks.HandleDirWatcherTask: repo.AddResult(%+v) result failed with %v\n", result, err)
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			result.Status = "failed"
			endTime := time.Now()
			result.EndTime = endTime.Unix()
			result.RunTime = endTime.Sub(st).Seconds()
			result.Error = fmt.Sprintf("%v", r)
			er := repo.UpdateResult(oid, result)
			if er != nil {
				log.Printf("tasks.HandleDirWatcherTask: Saving failed result caused %v\n during recovery", er)
			}
			log.Printf("tasks.HandleDirWatcherTask: recovered from %v while processing task with input %+v\n", r, c)
		}
	}()
	prevRes, err := repo.GetLastResult()
	if err != nil {
		log.Printf("tasks.HandleDirWatcherTask: repo.GetLastResult failed due to %v\n", err)
		panic(err)
	}
	report := WatchDirectory(os.Getenv("WATCHDIR"), c.MagicWord, prevRes.Files)
	endTime := time.Now()
	runTime := endTime.Sub(st).Seconds()
	result.EndTime = endTime.Unix()
	result.RunTime = runTime
	result.NewFiles = report.NewFiles
	result.Files = report.Files
	result.RemovedFiles = report.RemovedFiles
	result.Occurrence = report.Occurrence
	result.Status = "success"
	return repo.UpdateResult(oid, result)
}
