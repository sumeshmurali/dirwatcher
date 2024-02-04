/*
Package implements web server routers and handlers for the REST API
*/

package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/sumeshmurali/dirwatcher/internal"
)

var repo internal.Repository

const DEFAULT_PAGINATION_SIZE = 10

type ApiResponse struct {
	Success bool   `json:"success"` // indicates whether the request was successful or not
	Data    any    `json:"data"`    // result of the request. nil if there is no result
	Error   string `json:"error"`   // any error happened during request (success will be false if this field is non-nil)
}

func setupRouter() *gin.Engine {
	r := gin.Default()
	paginationSize, err := strconv.Atoi(os.Getenv("PAGINATION_SIZE"))
	if err != nil {
		log.Printf(`strconv.Atoi failed with error %v while converting %v to integer. 
			Setting to default value of %v instead\n`, err, os.Getenv("PAGINATION_SIZE"), DEFAULT_PAGINATION_SIZE)
		paginationSize = DEFAULT_PAGINATION_SIZE
	}
	asynqClient := asynq.NewClient(asynq.RedisClientOpt{Addr: os.Getenv("REDIS_ADDR")})

	r.GET("/results", func(ctx *gin.Context) {
		// Call to /results returns a list of tasks with details that has been processed
		// The endpoint supports an optional parameter "page" for pagination
		// Example - /result?page=2
		var page int64
		var apiResponse ApiResponse

		// parse the pagination parameter
		// note that the page is 0 indexed internally, however, for external users, it will be 1-indexed
		if p := ctx.Query("page"); p != "" {
			pi, err := strconv.Atoi(p)
			if err != nil || pi < 1 {
				log.Printf("GET /results: Parameter page=%v failed to parse with %v\n", p, err)
				apiResponse.Error = "page should be a positive integer (>1)"
				ctx.JSON(http.StatusBadRequest, apiResponse)
				return
			}
			page = int64(pi) - 1

		} else {
			// default page
			page = 0
		}

		results, err := repo.GetResults(page, int64(paginationSize))
		if err != nil {
			log.Printf("GET /results: repo.GetResults(%v, %v) failed with %v\n", page, paginationSize, err)
			apiResponse.Error = "Internal Server Error"
			ctx.JSON(http.StatusInternalServerError, apiResponse)
			return
		}
		apiResponse.Data = results
		apiResponse.Success = true
		ctx.JSON(http.StatusOK, apiResponse)
	})
	r.GET("/task-config", func(ctx *gin.Context) {
		// Call to /task-config (method GET) will return the existing task configuration
		var apiResponse ApiResponse
		taskConfig, err := repo.GetTaskConfig()
		if err != nil {
			log.Printf("GET /task-config: repo.GetTaskConfig() failed with %v\n", err)
			apiResponse.Error = "Internal Server Error"
			ctx.JSON(http.StatusInternalServerError, apiResponse)
		} else {
			apiResponse.Success = true
			apiResponse.Data = taskConfig
			ctx.JSON(http.StatusOK, apiResponse)
		}
	})
	r.POST("/task-config", func(ctx *gin.Context) {
		// Call to /task-config (method POST) with new configuration will update the configuration
		var newTaskConfig internal.TaskConfig
		var apiResponse ApiResponse

		if err := ctx.Bind(&newTaskConfig); err == nil {
			err := repo.PutTaskConfig(newTaskConfig)
			if err != nil {
				log.Printf("POST /task-config: repo.PutTaskConfig(%+v) failed with %v\n", newTaskConfig, err)
				apiResponse.Error = "Internal Server Error"
				ctx.JSON(http.StatusInternalServerError, apiResponse)
			} else {
				apiResponse.Success = true
				ctx.JSON(http.StatusCreated, apiResponse)
			}
		} else {
			log.Printf("POST /task-config: ctx.Bind on payload caused %v\n", err)
			apiResponse.Error = "Invalid Request Body"
			ctx.JSON(http.StatusBadRequest, apiResponse)
		}
	})
	r.GET("/trigger", func(ctx *gin.Context) {
		// Call to /trigger will initiate a new task run
		taskConfig, err := repo.GetTaskConfig()
		var apiResponse ApiResponse
		if err != nil {
			log.Printf("GET /trigger: repo.GetTaskConfig() caused %v\n", err)
			apiResponse.Error = "Internal Server Error"
			ctx.JSON(http.StatusBadRequest, apiResponse)
			return
		}
		p, err := json.Marshal(taskConfig)
		if err != nil {
			log.Printf("GET /trigger: json.Marshal(%+v) caused %v", taskConfig, err)
			apiResponse.Error = "Internal Server Error"
			ctx.JSON(http.StatusBadRequest, apiResponse)
			return
		}
		task := asynq.NewTask(internal.DirWatcherTask, p)
		_, err = asynqClient.Enqueue(task)
		if err != nil {
			log.Printf("GET /trigger: asynqClient.Enqueue(%+v) caused %v", task, err)
			apiResponse.Error = "Internal Server Error"
			ctx.JSON(http.StatusBadRequest, apiResponse)
			return
		}
		apiResponse.Success = true
		ctx.JSON(http.StatusOK, apiResponse)
	})

	return r
}

func main() {
	// loads the enviornment
	internal.LoadEnv()

	var err error
	repo, err = internal.NewRepository()
	if err != nil {
		log.Fatal(err)
	}
	defer repo.Close()

	r := setupRouter()
	r.Run(os.Getenv("API_ADDR"))
}
