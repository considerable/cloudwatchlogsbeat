package beater

import (
	"errors"
	"fmt"
	"time"

	"github.com/e-travel/cloudwatchlogsbeat/config"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs/cloudwatchlogsiface"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/publisher"
)

// Our cloud beat
type Cloudwatchlogsbeat struct {
	// Used to terminate process
	Done chan struct{}

	// Configuration
	Config config.Config

	// Beat publisher client
	Client publisher.Client

	// Beat persistence layer
	Registry Registry

	// Client to amazon cloudwatch logs API
	AWSClient cloudwatchlogsiface.CloudWatchLogsAPI

	// AWS client session
	Session *session.Session

	// the monitoring manager
	Manager *GroupManager
}

// Creates a new cloudwatchlogsbeat
func New(b *beat.Beat, cfg *common.Config) (beat.Beater, error) {
	// Read configuration
	configuration := config.Config{}
	if err := cfg.Unpack(&configuration); err != nil {
		return nil, fmt.Errorf("Error reading config file: %v", err)
	}

	// Create AWS session
	if configuration.AWSRegion == "" {
		configuration.AWSRegion = "eu-west-1"
	}
	sess := session.Must(session.NewSession(&aws.Config{
		Retryer: client.DefaultRetryer{NumMaxRetries: 10},
		Region:  aws.String(configuration.AWSRegion),
	}))

	// Create cloudwatch session
	svc := cloudwatchlogs.New(sess)

	// Create beat registry
	var registry Registry
	if configuration.StreamStateStorage == "" {
		configuration.StreamStateStorage = config.InMemoryStreamStateStorage
	}
	switch configuration.StreamStateStorage {
	case config.S3StreamStateStorage:
		if configuration.S3BucketName == "" {
			Fatal(errors.New(fmt.Sprintf("Please specify an s3 bucket for the requested registry")))
		}
		logp.Info("Using S3 Registry for state persistence.")
		registry = NewS3Registry(s3.New(sess), configuration.S3BucketName)
	case config.SqliteStreamStateStorage:
		if configuration.SqliteFilePath == "" {
			Fatal(errors.New(fmt.Sprintf("Please specify the path to the sqlite DB for the requested registry")))
		}
		logp.Info("Using sqlite registry for state persistence.")
		registry = NewSqliteRegistry(configuration.SqliteFilePath)
	case config.InMemoryStreamStateStorage:
		logp.Info("Using Dummy Registry. No state will be persisted.")
		registry = &DummyRegistry{}
	default:
		Fatal(errors.New(fmt.Sprintf("Invalid stream state storage: %s", configuration.StreamStateStorage)))
	}

	// Create instance
	beat := &Cloudwatchlogsbeat{
		Done:      make(chan struct{}),
		Config:    configuration,
		Session:   sess,
		AWSClient: svc,
		Registry:  registry,
	}

	beat.Manager = NewGroupManager(beat)

	// Validate configuration
	beat.ValidateConfig()

	return beat, nil
}

// Runs continuously our cloud beat
func (beat *Cloudwatchlogsbeat) Run(b *beat.Beat) error {
	logp.Info("cloudwatchlogsbeat is running! Hit CTRL-C to stop it.")

	beat.Client = b.Publisher.Connect()
	ticker := time.NewTicker(beat.Config.GroupRefreshPeriod)

	for {
		select {
		case <-beat.Done:
			return nil
		case <-ticker.C:
		}
		beat.Process()
	}
}

// Stops beat client
func (beat *Cloudwatchlogsbeat) Stop() {
	beat.Client.Close()
	close(beat.Done)
}

// Processes each prospector of our cloud beat
func (beat *Cloudwatchlogsbeat) Process() {
	beat.Manager.Monitor()
}

// Performs basic validation for our configuration, like our
// regular expressions are valid, ...
func (beat *Cloudwatchlogsbeat) ValidateConfig() {
	for _, prospector := range beat.Config.Prospectors {
		ValidateMultiline(prospector.Multiline)
	}
}
