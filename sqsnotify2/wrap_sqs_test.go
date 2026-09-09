package sqsnotify2

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

type mockSQSClient struct {
	getQueueUrlFn        func(ctx context.Context, params *sqs.GetQueueUrlInput, optFns ...func(*sqs.Options)) (*sqs.GetQueueUrlOutput, error)
	createQueueFn        func(ctx context.Context, params *sqs.CreateQueueInput, optFns ...func(*sqs.Options)) (*sqs.CreateQueueOutput, error)
	receiveMessageFn     func(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	deleteMessageBatchFn func(ctx context.Context, params *sqs.DeleteMessageBatchInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageBatchOutput, error)
}

func (m *mockSQSClient) GetQueueUrl(ctx context.Context, params *sqs.GetQueueUrlInput, optFns ...func(*sqs.Options)) (*sqs.GetQueueUrlOutput, error) {
	if m.getQueueUrlFn != nil {
		return m.getQueueUrlFn(ctx, params, optFns...)
	}
	return nil, errors.New("GetQueueUrl not implemented")
}

func (m *mockSQSClient) CreateQueue(ctx context.Context, params *sqs.CreateQueueInput, optFns ...func(*sqs.Options)) (*sqs.CreateQueueOutput, error) {
	if m.createQueueFn != nil {
		return m.createQueueFn(ctx, params, optFns...)
	}
	return nil, errors.New("CreateQueue not implemented")
}

func (m *mockSQSClient) ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	if m.receiveMessageFn != nil {
		return m.receiveMessageFn(ctx, params, optFns...)
	}
	return nil, errors.New("ReceiveMessage not implemented")
}

func (m *mockSQSClient) DeleteMessageBatch(ctx context.Context, params *sqs.DeleteMessageBatchInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageBatchOutput, error) {
	if m.deleteMessageBatchFn != nil {
		return m.deleteMessageBatchFn(ctx, params, optFns...)
	}
	return nil, errors.New("DeleteMessageBatch not implemented")
}

func TestGetQueueURL(t *testing.T) {
	ctx := context.Background()

	t.Run("Queue exists", func(t *testing.T) {
		mock := &mockSQSClient{
			getQueueUrlFn: func(ctx context.Context, params *sqs.GetQueueUrlInput, optFns ...func(*sqs.Options)) (*sqs.GetQueueUrlOutput, error) {
				if aws.ToString(params.QueueName) != "test-queue" {
					t.Fatalf("unexpected queue name: %s", aws.ToString(params.QueueName))
				}
				return &sqs.GetQueueUrlOutput{
					QueueUrl: aws.String("https://sqs.us-east-1.amazonaws.com/123456789012/test-queue"),
				}, nil
			},
		}

		url, err := getQueueURL(ctx, mock, "test-queue", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if aws.ToString(url) != "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue" {
			t.Fatalf("unexpected queue url: %s", aws.ToString(url))
		}
	})

	t.Run("Queue does not exist and create is true", func(t *testing.T) {
		mock := &mockSQSClient{
			getQueueUrlFn: func(ctx context.Context, params *sqs.GetQueueUrlInput, optFns ...func(*sqs.Options)) (*sqs.GetQueueUrlOutput, error) {
				return nil, &types.QueueDoesNotExist{Message: aws.String("The specified queue does not exist.")}
			},
			createQueueFn: func(ctx context.Context, params *sqs.CreateQueueInput, optFns ...func(*sqs.Options)) (*sqs.CreateQueueOutput, error) {
				if aws.ToString(params.QueueName) != "new-queue" {
					t.Fatalf("unexpected queue name: %s", aws.ToString(params.QueueName))
				}
				return &sqs.CreateQueueOutput{
					QueueUrl: aws.String("https://sqs.us-east-1.amazonaws.com/123456789012/new-queue"),
				}, nil
			},
		}

		url, err := getQueueURL(ctx, mock, "new-queue", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if aws.ToString(url) != "https://sqs.us-east-1.amazonaws.com/123456789012/new-queue" {
			t.Fatalf("unexpected queue url: %s", aws.ToString(url))
		}
	})

	t.Run("Queue does not exist and create is false", func(t *testing.T) {
		mock := &mockSQSClient{
			getQueueUrlFn: func(ctx context.Context, params *sqs.GetQueueUrlInput, optFns ...func(*sqs.Options)) (*sqs.GetQueueUrlOutput, error) {
				return nil, &types.QueueDoesNotExist{Message: aws.String("The specified queue does not exist.")}
			},
		}

		_, err := getQueueURL(ctx, mock, "nonexistent-queue", false)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !isQueueDoesNotExist(err) {
			t.Fatalf("expected QueueDoesNotExist error, got %v", err)
		}
	})
}

func TestReceiveMessages(t *testing.T) {
	ctx := context.Background()

	mock := &mockSQSClient{
		receiveMessageFn: func(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
			if params.MaxNumberOfMessages != 10 {
				t.Fatalf("expected MaxNumberOfMessages 10, got %d", params.MaxNumberOfMessages)
			}
			if params.WaitTimeSeconds != 20 {
				t.Fatalf("expected WaitTimeSeconds 20, got %d", params.WaitTimeSeconds)
			}
			return &sqs.ReceiveMessageOutput{
				Messages: []types.Message{
					{MessageId: aws.String("msg-1"), Body: aws.String("hello")},
				},
			}, nil
		},
	}

	queueURL := aws.String("https://sqs.us-east-1.amazonaws.com/123456789012/test-queue")
	wt := int32(20)
	msgs, err := receiveMessages(ctx, mock, queueURL, 10, &wt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if aws.ToString(msgs[0].Body) != "hello" {
		t.Fatalf("expected body 'hello', got %s", aws.ToString(msgs[0].Body))
	}
}

func TestDeleteMessages(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		mock := &mockSQSClient{
			deleteMessageBatchFn: func(ctx context.Context, params *sqs.DeleteMessageBatchInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageBatchOutput, error) {
				if len(params.Entries) != 1 {
					t.Fatalf("expected 1 entry, got %d", len(params.Entries))
				}
				return &sqs.DeleteMessageBatchOutput{}, nil
			},
		}

		queueURL := aws.String("https://sqs.us-east-1.amazonaws.com/123456789012/test-queue")
		entries := []types.DeleteMessageBatchRequestEntry{
			{Id: aws.String("1"), ReceiptHandle: aws.String("handle-1")},
		}
		err := deleteMessages(ctx, mock, queueURL, entries)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("Failure", func(t *testing.T) {
		mock := &mockSQSClient{
			deleteMessageBatchFn: func(ctx context.Context, params *sqs.DeleteMessageBatchInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageBatchOutput, error) {
				return &sqs.DeleteMessageBatchOutput{
					Failed: []types.BatchResultErrorEntry{
						{Id: aws.String("1"), Code: aws.String("ReceiptHandleIsInvalid")},
					},
				}, nil
			},
		}

		queueURL := aws.String("https://sqs.us-east-1.amazonaws.com/123456789012/test-queue")
		entries := []types.DeleteMessageBatchRequestEntry{
			{Id: aws.String("1"), ReceiptHandle: aws.String("handle-1")},
		}
		err := deleteMessages(ctx, mock, queueURL, entries)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
