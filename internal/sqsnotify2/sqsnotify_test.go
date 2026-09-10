package sqsnotify2

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Admiral-Piett/goaws/app/models"
	"github.com/Admiral-Piett/goaws/app/servertest"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/koron/sqs-notify/internal/cache"
)

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	body, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(2)
	}
	if strings.Contains(string(body), "FAIL") {
		os.Exit(1)
	}
	fmt.Printf("HELPER_OUTPUT: %s\n", string(body))
}

func setupGoAWSServer(t *testing.T) (*servertest.Server, *sqs.Client) {
	t.Helper()

	srv, err := servertest.New("127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start goaws servertest: %v", err)
	}

	u, err := url.Parse(srv.URL())
	if err != nil {
		srv.Quit()
		t.Fatalf("failed to parse server URL: %v", err)
	}

	models.CurrentEnvironment.Host = u.Hostname()
	models.CurrentEnvironment.Port = u.Port()
	models.CurrentEnvironment.Region = "us-east-1"

	ctx := context.Background()
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
	)
	if err != nil {
		srv.Quit()
		t.Fatalf("failed to load aws config: %v", err)
	}

	sqsClient := sqs.NewFromConfig(awsCfg, func(o *sqs.Options) {
		o.BaseEndpoint = aws.String(srv.URL())
	})

	t.Cleanup(func() { srv.Quit() })
	return srv, sqsClient
}

func TestSQSNotify(t *testing.T) {
	os.Setenv("GO_WANT_HELPER_PROCESS", "1")
	os.Setenv("AWS_ACCESS_KEY_ID", "mock_access_key")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "mock_secret_key")
	os.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Cleanup(func() {
		os.Unsetenv("GO_WANT_HELPER_PROCESS")
		os.Unsetenv("AWS_ACCESS_KEY_ID")
		os.Unsetenv("AWS_SECRET_ACCESS_KEY")
		os.Unsetenv("AWS_EC2_METADATA_DISABLED")
	})

	t.Run("CreateQueue and Process Message with Succeed Policy", func(t *testing.T) {
		srv, sqsClient := setupGoAWSServer(t)

		queueName := "test-goaws-create-queue"
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		wt := int64(1)
		cfg := NewConfig()
		cfg.Endpoint = srv.URL()
		cfg.Region = "us-east-1"
		cfg.QueueName = queueName
		cfg.CreateQueue = true
		cfg.CmdName = os.Args[0]
		cfg.CmdArgs = []string{"-test.run=TestHelperProcess"}
		cfg.RemovePolicy = Succeed
		cfg.WaitTime = &wt

		sn := New(cfg)

		runCtx, runCancel := context.WithCancel(ctx)
		defer runCancel()

		errCh := make(chan error, 1)
		go func() {
			errCh <- sn.Run(runCtx, cache.NewMemoryCache(10))
		}()

		// Wait for queue creation by SQSNotify
		var qURL string
		for i := 0; i < 50; i++ {
			qURLRes, err := sqsClient.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
				QueueName: aws.String(queueName),
			})
			if err == nil && qURLRes.QueueUrl != nil {
				qURL = *qURLRes.QueueUrl
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		if qURL == "" {
			t.Fatalf("queue was not created in time")
		}

		// Send message to the created queue
		_, err := sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
			QueueUrl:    aws.String(qURL),
			MessageBody: aws.String("hello goaws succeed"),
		})
		if err != nil {
			t.Fatalf("failed to send message: %v", err)
		}

		// Wait for message processing
		time.Sleep(500 * time.Millisecond)

		runCancel()
		runErr := <-errCh
		if runErr != nil && !errorsIsCanceled(runErr) {
			t.Fatalf("SQSNotify.Run returned unexpected error: %v", runErr)
		}
	})

	t.Run("Existing Queue and BeforeExecution Policy", func(t *testing.T) {
		srv, sqsClient := setupGoAWSServer(t)

		queueName := "test-goaws-before-execution"
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		createRes, err := sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{
			QueueName: aws.String(queueName),
		})
		if err != nil {
			t.Fatalf("failed to create queue: %v", err)
		}

		wt := int64(1)
		cfg := NewConfig()
		cfg.Endpoint = srv.URL()
		cfg.Region = "us-east-1"
		cfg.QueueName = queueName
		cfg.CreateQueue = false
		cfg.CmdName = os.Args[0]
		cfg.CmdArgs = []string{"-test.run=TestHelperProcess"}
		cfg.RemovePolicy = BeforeExecution
		cfg.WaitTime = &wt

		sn := New(cfg)

		runCtx, runCancel := context.WithCancel(ctx)
		defer runCancel()

		errCh := make(chan error, 1)
		go func() {
			errCh <- sn.Run(runCtx, cache.NewMemoryCache(10))
		}()

		time.Sleep(100 * time.Millisecond)

		_, err = sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
			QueueUrl:    createRes.QueueUrl,
			MessageBody: aws.String("hello goaws before execution"),
		})
		if err != nil {
			t.Fatalf("failed to send message: %v", err)
		}

		time.Sleep(500 * time.Millisecond)

		// TODO: Confirm that actual message deletion is occurring.

		runCancel()
		runErr := <-errCh
		if runErr != nil && !errorsIsCanceled(runErr) {
			t.Fatalf("SQSNotify.Run returned unexpected error: %v", runErr)
		}
	})

	t.Run("Existing Queue and IgnoreFailure Policy", func(t *testing.T) {
		srv, sqsClient := setupGoAWSServer(t)

		queueName := "test-goaws-ignore-failure"
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		createRes, err := sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{
			QueueName: aws.String(queueName),
		})
		if err != nil {
			t.Fatalf("failed to create queue: %v", err)
		}

		wt := int64(1)
		cfg := NewConfig()
		cfg.Endpoint = srv.URL()
		cfg.Region = "us-east-1"
		cfg.QueueName = queueName
		cfg.CreateQueue = false
		cfg.CmdName = os.Args[0]
		cfg.CmdArgs = []string{"-test.run=TestHelperProcess"}
		cfg.RemovePolicy = IgnoreFailure
		cfg.WaitTime = &wt

		sn := New(cfg)

		runCtx, runCancel := context.WithCancel(ctx)
		defer runCancel()

		errCh := make(chan error, 1)
		go func() {
			errCh <- sn.Run(runCtx, cache.NewMemoryCache(10))
		}()

		time.Sleep(100 * time.Millisecond)

		_, err = sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
			QueueUrl:    createRes.QueueUrl,
			MessageBody: aws.String("FAIL - should ignore failure"),
		})
		if err != nil {
			t.Fatalf("failed to send message: %v", err)
		}

		time.Sleep(500 * time.Millisecond)

		// TODO: Confirm that actual message deletion is occurring.

		runCancel()
		runErr := <-errCh
		if runErr != nil && !errorsIsCanceled(runErr) {
			t.Fatalf("SQSNotify.Run returned unexpected error: %v", runErr)
		}
	})
}

func errorsIsCanceled(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "canceled") || strings.Contains(err.Error(), "context canceled")
}
