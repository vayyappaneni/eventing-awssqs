/*
Copyright 2018 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package adapter

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"go.uber.org/zap"
	"golang.org/x/net/context"
	sourcesv1alpha1 "knative.dev/eventing-awssqs/pkg/apis/sources/v1alpha1"
	"knative.dev/pkg/logging"
)

// Adapter implements the AWS SQS adapter to deliver SQS messages from
// an SQS queue to a Sink.
type Adapter struct {

	// QueueURL is the AWS SQS URL that we're polling messages from
	QueueURL string

	// SinkURI is the URI messages will be forwarded on to.
	SinkURI string

	// CredsFile is the full path of the AWS credentials file
	CredsFile string

	// MaxBatchSize is the max Batch size to be polled form SQS
	MaxBatchSize string

	// SendBatchedResponse is a flag which if enabled will send all messages received in one HTTP event
	SendBatchedResponse string

	// OnFailedPollWaitSecs determines the interval to wait after a
	// failed poll before making another one
	OnFailedPollWaitSecs string

	// WaitTimeSeconds Controls the maximum time to wait in the poll performed with
	// ReceiveMessageWithContext.  If there are no messages in the
	// given secs, the call times out and returns control to us.
	WaitTimeSeconds string

	// Client sends cloudevents to the target.
	client cloudevents.Client
}

const (
	defaultSendBatchedResponse = false

	defaultMaxBatchSize = 10

	defaultOnFailedPollWaitSecs = 2

	defaultWaitTimeSeconds = 3
)

// getRegion takes an AWS SQS URL and extracts the region from it
// e.g. URLs have this form:
// https://sqs.<region>.amazonaws.com/<account_id>/<queue_name>
// See
// https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-general-identifiers.html
// for reference.  Note that AWS does not make any promises re. url
// structure although it feels reasonable to rely on it at this point
// rather than add an additional `region` parameter to the spec that
// will now be redundant most of the time.
func getRegion(url string) (string, error) {
	parts := strings.Split(url, ".")

	if len(parts) < 2 {
		err := fmt.Errorf("QueueURL does not look correct: %s", url)
		return "", err
	}
	return parts[1], nil
}

// Initialize cloudevent client
func (a *Adapter) initClient() error {
	if a.client == nil {
		var err error
		if a.client, err = cloudevents.NewDefaultClient(); err != nil {
			return err
		}
	}
	return nil
}

func (a *Adapter) Start(ctx context.Context, stopCh <-chan struct{}) error {

	logger := logging.FromContext(ctx)

	logger.Info("Starting with config: ", zap.Any("adapter", a))

	if err := a.initClient(); err != nil {
		logger.Error("Failed to create cloudevent client", zap.Error(err))
		return err
	}

	region, err := getRegion(a.QueueURL)
	if err != nil {
		logger.Error("Failed to parse region from queue URL", err)
		return err
	}

	var sess *session.Session

	if a.CredsFile == "" {
		sess = session.Must(session.NewSessionWithOptions(session.Options{
			Config: aws.Config{Region: &region},
		}))
	} else {
		sess = session.Must(session.NewSessionWithOptions(session.Options{
			SharedConfigState: session.SharedConfigDisable,
			Config:            aws.Config{Region: &region},
			SharedConfigFiles: []string{a.CredsFile},
		}))
	}

	q := sqs.New(sess)

	return a.pollLoop(ctx, q, stopCh)
}

func (a *Adapter) getDeleteMessageEntries(sqsMessages []*sqs.Message) (Entries []*sqs.DeleteMessageBatchRequestEntry) {
    var list []*sqs.DeleteMessageBatchRequestEntry
    for _, message := range sqsMessages {
        list = append(list, &sqs.DeleteMessageBatchRequestEntry {
        	Id:            message.MessageId,
        	ReceiptHandle: message.ReceiptHandle,
        })
    }
    return list
}

// pollLoop continuously polls from the given SQS queue until stopCh
// emits an element.  The
func (a *Adapter) pollLoop(ctx context.Context, q *sqs.SQS, stopCh <-chan struct{}) error {

	logger := logging.FromContext(ctx)


	maxBatchSize, err := strconv.ParseInt(a.MaxBatchSize,10,64)
	if err != nil {
		logger.Info("Could not Find or convert maxBatchSize from string to int. Defaulting to ", defaultMaxBatchSize, zap.Error(err))
		maxBatchSize = defaultMaxBatchSize
	}
	sendBatchedResponse, err := strconv.ParseBool(a.SendBatchedResponse)
	if err != nil {
		logger.Info("Could not Find or convert sendBatchedResponse from string to bool, Defaulting to", defaultSendBatchedResponse, zap.Error(err))
		sendBatchedResponse = defaultSendBatchedResponse
	}
	onFailedPollWaitSecs, err := strconv.ParseInt(a.OnFailedPollWaitSecs,10,0)
	if err != nil {
		logger.Info("Could not Find or convert onFailedPollWaitSecs from string to time.Duration. Defaulting to ", defaultOnFailedPollWaitSecs, zap.Error(err))
		onFailedPollWaitSecs = defaultOnFailedPollWaitSecs
	}
	waitTimeSeconds, err := strconv.ParseInt(a.WaitTimeSeconds,10,64)
	if err != nil {
		logger.Info("Could not Find or convert waitTimeSeconds from string to int. Defaulting to ", defaultWaitTimeSeconds, zap.Error(err))
		waitTimeSeconds = defaultWaitTimeSeconds
	}


	logger.Infof("value from configs: MaxBatchSize: %d, SendBatchedResponse: %b, OnFailedPollWaitSecs: %d, WaitTimeSeconds: %d", maxBatchSize, sendBatchedResponse, onFailedPollWaitSecs, waitTimeSeconds)


	for {
		select {
		case <-stopCh:
			logger.Info("Exiting")
			return nil
		default:
		}

		messages, err := poll(ctx, q, a.QueueURL, maxBatchSize, waitTimeSeconds)
		if err != nil {
			logger.Warn("Failed to poll from SQS queue", zap.Error(err))
			time.Sleep(time.Duration(onFailedPollWaitSecs) * time.Second)
			continue
		}

		if (sendBatchedResponse && len(messages) > 0){
			a.receiveMessages(ctx, messages, func() {
				_, err = q.DeleteMessageBatch(&sqs.DeleteMessageBatchInput{
					QueueUrl:      &a.QueueURL,
					Entries: a.getDeleteMessageEntries(messages),
				})
				if err != nil {
					// the only consequence is that the message will
					// get redelivered later, given that SQS is
					// at-least-once delivery. That should be
					// acceptable as "normal operation"
					logger.Error("Failed to delete messages", zap.Error(err))
				}
			})
		} else {
			for _, m := range messages {
				a.receiveMessage(ctx, m, func() {
					_, err = q.DeleteMessage(&sqs.DeleteMessageInput{
						QueueUrl:      &a.QueueURL,
						ReceiptHandle: m.ReceiptHandle,
					})
					if err != nil {
						// the only consequence is that the message will
						// get redelivered later, given that SQS is
						// at-least-once delivery. That should be
						// acceptable as "normal operation"
						logger.Error("Failed to delete message", zap.Error(err))
					}
				})
			}

		}
	}
}

// receiveMessage handles an incoming message from the AWS SQS queue,
// and forwards it to a Sink, calling `ack()` when the forwarding is
// successful.
func (a *Adapter) receiveMessage(ctx context.Context, m *sqs.Message, ack func()) {
	logger := logging.FromContext(ctx).With(zap.Any("eventID", m.MessageId)).With(zap.Any("sink", a.SinkURI))
	logger.Debugw("Received message from SQS:", zap.Any("message", m))

	ctx = cloudevents.ContextWithTarget(ctx, a.SinkURI)

	err := a.postMessage(ctx, logger, m)
	if err != nil {
		logger.Infof("Event delivery failed: %s", err)
	} else {
		logger.Debug("Message successfully posted to Sink")
		ack()
	}
}

// receiveMessages handles an incoming list of message from the AWS SQS queue,
// and forwards it to a Sink, calling `ack()` when the forwarding is
// successful.
func (a *Adapter) receiveMessages(ctx context.Context, messages []*sqs.Message, ack func()) {
	logger := logging.FromContext(ctx).With(zap.Any("eventID", messages[0].MessageId)).With(zap.Any("sink", a.SinkURI))
	logger.Debugw("Received messages from SQS:", zap.Any("messagesLength", len(messages)))

	ctx = cloudevents.ContextWithTarget(ctx, a.SinkURI)

	err := a.postMessages(ctx, logger, messages)
	if err != nil {
		logger.Infof("Event delivery failed: %s", err)
	} else {
		logger.Debug("Message successfully posted to Sink")
		ack()
	}
}

func (a *Adapter) makeEvent(m *sqs.Message) (*cloudevents.Event, error) {
	timestamp, err := strconv.ParseInt(*m.Attributes["SentTimestamp"], 10, 64)
	if err == nil {
		//Convert to nanoseconds as sqs SentTimestamp is millisecond
		timestamp = timestamp * int64(1000000)
	} else {
		timestamp = time.Now().UnixNano()
	}

	event := cloudevents.NewEvent(cloudevents.VersionV1)
	event.SetID(*m.MessageId)
	event.SetType(sourcesv1alpha1.AwsSqsSourceEventType)
	event.SetSource(cloudevents.ParseURIRef(a.QueueURL).String())
	event.SetTime(time.Unix(0, timestamp))

	if err := event.SetData(cloudevents.ApplicationJSON, m); err != nil {
		return nil, err
	}
	return &event, nil
}

func (a *Adapter) makeBatchEvent(messages []*sqs.Message) (*cloudevents.Event, error) {
	timestamp, err := strconv.ParseInt(*messages[0].Attributes["SentTimestamp"], 10, 64)
	if err == nil {
		//Convert to nanoseconds as sqs SentTimestamp is millisecond
		timestamp = timestamp * int64(1000000)
	} else {
		timestamp = time.Now().UnixNano()
	}

	event := cloudevents.NewEvent(cloudevents.VersionV1)
	event.SetID(*messages[0].MessageId)
	event.SetType(sourcesv1alpha1.AwsSqsSourceEventType)
	event.SetSource(cloudevents.ParseURIRef(a.QueueURL).String())
	event.SetTime(time.Unix(0, timestamp))

	if err := event.SetData(cloudevents.ApplicationJSON, messages); err != nil {
		return nil, err
	}
	return &event, nil
}

// postMessage sends an SQS event to the SinkURI
func (a *Adapter) postMessage(ctx context.Context, logger *zap.SugaredLogger, m *sqs.Message) error {
	event, err := a.makeEvent(m)

	if err != nil {
		logger.Error("Cloud Event creation error", zap.Error(err))
		return err
	}
	if result := a.client.Send(ctx, *event); !cloudevents.IsACK(result) {
		logger.Error("Cloud Event delivery error", zap.Error(result))
		return result
	}
	return nil
}
// postMessages sends an array of SQS events to the SinkURI
func (a *Adapter) postMessages(ctx context.Context, logger *zap.SugaredLogger, messages []*sqs.Message) error {
	event, err := a.makeBatchEvent(messages)

	if err != nil {
		logger.Error("Cloud Event creation error", zap.Error(err))
		return err
	}
	if result := a.client.Send(ctx, *event); !cloudevents.IsACK(result) {
		logger.Error("Cloud Event delivery error", zap.Error(result))
		return result
	}
	return nil
}

// poll reads messages from the queue in batches of a given maximum size.
func poll(ctx context.Context, q *sqs.SQS, url string, maxBatchSize int64, waitTimeSeconds int64) ([]*sqs.Message, error) {

	result, err := q.ReceiveMessageWithContext(ctx, &sqs.ReceiveMessageInput{
		AttributeNames: []*string{
			aws.String(sqs.MessageSystemAttributeNameSentTimestamp),
		},
		MessageAttributeNames: []*string{
			aws.String(sqs.QueueAttributeNameAll),
		},
		QueueUrl: &url,
		// Maximum size of the batch of messages returned from the poll.
		MaxNumberOfMessages: aws.Int64(maxBatchSize),
		// Controls the maximum time to wait in the poll performed with
		// ReceiveMessageWithContext.  If there are no messages in the
		// given secs, the call times out and returns control to us.
		// TODO: expose this as ENV variable
		WaitTimeSeconds: aws.Int64(waitTimeSeconds),
	})

	if err != nil {
		return []*sqs.Message{}, err
	}

	return result.Messages, nil
}
