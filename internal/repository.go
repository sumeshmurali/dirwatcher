/*
repository package uses the Repository design pattern to abstract the database layer from the service layer
*/

package internal

import (
	"context"
	"os"
	"strconv"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Repository interface provides necessary methods to access the data layer
// TODO decouple Repository.AddResult and UpdateResult from primitive.ObjectID
// TODO decouple Repository.Init from the MongoConfig
type Repository interface {
	// GetTaskConfig returns the current task configuration
	GetTaskConfig() (TaskConfig, error)
	// PutTaskConfig sets the given task configuration in the database
	PutTaskConfig(TaskConfig) error
	// GetResults returns a []Result containing previous task results
	// Pagination can be done with incrementing the page parameter
	// More results can be fetched by increasing the size parameter
	GetResults(page int64, size int64) ([]Result, error)
	// GetLastResult returns the most recent succesfully completed run
	GetLastResult() (Result, error)
	// AddResult inserts the new result into the database and returns an id for reference
	AddResult(result Result) (primitive.ObjectID, error)
	// UpdateResult updates an existing result filtered by using the id
	UpdateResult(id primitive.ObjectID, result Result) error
	// Init initializes the DB with the given configuration
	Init(config MongoConfig) error
	// Close closes/disconnects/clears the connection to the database
	Close()
}

type TaskConfig struct {
	Interval  float64 `json:"interval,omitempty" bson:"interval,omitempty"` // interval for scheduling a task
	MagicWord string  `json:"magicWord" bson:"magicWord"`                   // word to search in the directory
}

type Result struct {
	StartTime    int64    `json:"startTime" bson:"startTime"`       // Start Time of the task in epoch
	EndTime      int64    `json:"endTime" bson:"endTime"`           // End Time of the task in epoch
	RunTime      float64  `json:"runTime" bson:"runTime,truncate"`  // Total run time in seconds
	Status       string   `json:"status" bson:"status"`             // Status of the task
	MagicWord    string   `json:"magicWord" bson:"magicWord"`       // Word that was searched in the directory
	Occurrence   int      `json:"occurences" bson:"occurences"`     // Number of occurences of MagicWord
	Files        []string `json:"files" bson:"files"`               // Files found in the directory
	NewFiles     []string `json:"newFiles" bson:"newFiles"`         // New Files found in the directory
	RemovedFiles []string `json:"removedFiles" bson:"removedFiles"` // Files that are deleted from the directory
	Error        string   `json:"error" bson:"error"`               // Error occured during the task execution
}

type MongoConfig struct {
	Url      string // URI to the mongo server
	Database string // Database name to use
}

type MongoRepository struct {
	client *mongo.Client
	db     *mongo.Database
}

// Init creats and connects the Mongo client to the Database
func (m *MongoRepository) Init(config MongoConfig) error {
	c, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(config.Url))
	if err != nil {
		return err
	}
	// creating database
	m.client = c
	m.db = c.Database(config.Database)

	// creating "tasks" collection
	// creating a capped collection of one document to store the task config.
	// This has a nice side effect of not having to delete the previous
	// config when you need to replace it. Mongo automatically replaces it for you
	err = m.db.CreateCollection(
		context.TODO(),
		"tasks",
		options.CreateCollection().
			SetCapped(true).
			SetSizeInBytes(10000).
			SetMaxDocuments(1))
	if err != nil {
		if _, ok := err.(mongo.CommandError); ok {
			// collection "tasks" already exists. We can ignore this
		} else {
			// return the error
			return err
		}
	}
	return nil
}

// Close disconnects the client from the server
func (m *MongoRepository) Close() {
	m.client.Disconnect(context.TODO())
}

// GetTaskConfig returns the current task configuration
func (m *MongoRepository) GetTaskConfig() (TaskConfig, error) {
	coll := m.db.Collection("tasks")
	var taskConfig TaskConfig
	// There is only one config available always, so no need to apply any filter
	res := coll.FindOne(context.TODO(), bson.D{})
	err := res.Decode(&taskConfig)
	if err == nil {
		return taskConfig, nil
	}
	// if there is some error, check if it is a no documents error
	if err == mongo.ErrNoDocuments {
		// If the error is no documents error, this means there are no configurations in the the database
		// Use/Set a default config when calling the function before setting a task config
		interval, err := strconv.ParseFloat(os.Getenv("DEFAULT_TASK_INTERVAL"), 64)
		if err != nil {
			return taskConfig, err
		}
		taskConfig = TaskConfig{Interval: interval, MagicWord: os.Getenv("DEFAULT_SEARCH_STRING")}
		_, err = coll.InsertOne(context.TODO(), taskConfig)
		return taskConfig, err
	}

	return taskConfig, err
}

// PutTaskConfig replaces the existing task configuration with the provided one
func (m *MongoRepository) PutTaskConfig(taskConfig TaskConfig) error {

	coll := m.db.Collection("tasks")
	_, err := coll.InsertOne(context.TODO(), taskConfig)
	if err != nil {
		return err
	}
	return nil
}

// GetResults return a limited set of most recent results from the database.
// Page parameter controls the pagination.
// Size Parameter limits the results per call.
func (m *MongoRepository) GetResults(page int64, size int64) ([]Result, error) {
	coll := m.db.Collection("results")
	opts := options.Find().SetSort(bson.D{{Key: "startTime", Value: -1}}).SetLimit(size).SetSkip(page * size)
	cur, err := coll.Find(context.TODO(), bson.D{}, opts)
	if err != nil {
		return nil, err
	}
	var results []Result
	err = cur.All(context.TODO(), &results)
	return results, err
}

// AddResult adds a new result to the data base and returns its id
func (m *MongoRepository) AddResult(result Result) (primitive.ObjectID, error) {
	coll := m.db.Collection("results")
	res, err := coll.InsertOne(context.TODO(), result)
	return res.InsertedID.(primitive.ObjectID), err
}

// UpdateResult updates an existing result filtered by id parameter
func (m *MongoRepository) UpdateResult(id primitive.ObjectID, result Result) error {
	coll := m.db.Collection("results")
	update := bson.D{{Key: "$set", Value: result}}
	_, err := coll.UpdateOne(context.TODO(), bson.D{{Key: "_id", Value: id}}, update)
	return err
}

// GetLastResult returns the most recent successfully executed task's result
func (m *MongoRepository) GetLastResult() (Result, error) {
	coll := m.db.Collection("results")

	opts := options.FindOne().SetSort(bson.D{{Key: "endTime", Value: -1}})
	res := coll.FindOne(context.TODO(), bson.D{}, opts)
	var r Result
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			return r, nil
		}
		return r, err
	}
	res.Decode(&r)
	return r, nil
}

// NewRepository returns a repository of the configured database.
// See Repository for all the supported methods
//
// Any error occured during initialization is returned with nil Repository
func NewRepository() (*MongoRepository, error) {

	config := MongoConfig{
		Url:      os.Getenv("MONGO_URI"),
		Database: os.Getenv("MONGO_DB"),
	}
	repo := MongoRepository{}
	err := repo.Init(config)
	return &repo, err
}
