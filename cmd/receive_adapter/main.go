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

package main

import (
	"flag"
	"log"
	"os"

	awssqs "knative.dev/eventing-awssqs/pkg/adapter"
	"knative.dev/pkg/signals"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"golang.org/x/net/context"
)

const (
	// envSinkURI for messages.
	envSinkURI = "K_SINK"

	// envQueueURL is the URL of the SQS queue to consume messages from.
	envQueueURL = "AWS_SQS_URL"

	// envCredsFile is the path of the AWS credentials file
	envCredsFile = "AWS_APPLICATION_CREDENTIALS"

	envMaxBatchSize = "AWS_SQS_MAX_BATCH_SIZE"

	envSendBatchedResponse = "AWS_SQS_SEND_BATCH_RESPONSE"

	envOnFailedPollWaitSecs = "AWS_SQS_POLL_FAILED_WAIT_TIME"

	envWaitTimeSeconds = "AWS_SQS_WAIT_TIME_SECONDS"
)

func getRequiredEnv(envKey string) string {
	val, defined := os.LookupEnv(envKey)
	if !defined {
		log.Fatalf("required environment variable not defined '%s'", envKey)
	}
	return val
}

func main() {
	flag.Parse()

	ctx := context.Background()
	logCfg := zap.NewProductionConfig()
	logCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	logger, err := logCfg.Build()
	if err != nil {
		log.Fatalf("Unable to create logger: %v", err)
	}

	adapter := &awssqs.Adapter{
		QueueURL:             getRequiredEnv(envQueueURL),
		SinkURI:              getRequiredEnv(envSinkURI),
		CredsFile:            os.Getenv(envCredsFile),
		OnFailedPollWaitSecs: os.Getenv(envOnFailedPollWaitSecs),
		MaxBatchSize:         os.Getenv(envMaxBatchSize),
		SendBatchedResponse:  os.Getenv(envSendBatchedResponse),
		WaitTimeSeconds:      os.Getenv(envWaitTimeSeconds),
	}

	logger.Info("Starting AWS SQS Receive Adapter.", zap.Any("adapter", adapter))
	stopCh := signals.SetupSignalHandler()
	if err := adapter.Start(ctx, stopCh); err != nil {
		logger.Fatal("failed to start adapter: ", zap.Error(err))
	}
}
