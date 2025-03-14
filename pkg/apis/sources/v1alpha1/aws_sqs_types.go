/*
Copyright 2019 The Knative Authors

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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

// +genclient
// +genreconciler
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AwsSqsSource is the Schema for the AWS SQS API
// +k8s:openapi-gen=true
type AwsSqsSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AwsSqsSourceSpec   `json:"spec,omitempty"`
	Status AwsSqsSourceStatus `json:"status,omitempty"`
}

// Check that AwsSqsSource can be validated and can be defaulted.
var _ runtime.Object = (*AwsSqsSource)(nil)

// Check that the type conforms to the duck Knative Resource shape.
var _ duckv1.KRShaped = (*AwsSqsSource)(nil)

// AwsSqsSourceSpec defines the desired state of the source.
type AwsSqsSourceSpec struct {
	// QueueURL of the SQS queue that we will poll from.
	QueueURL string `json:"queueUrl"`

	// AwsCredsSecret is the credential to use to poll the AWS SQS
	// +optional
	AwsCredsSecret *corev1.SecretKeySelector `json:"awsCredsSecret,omitempty"`

	// Annotations to add to the pod, mostly used for Kube2IAM role
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Number of replicas for the SQS Adapter
    // +optional
    Replicas int32 `json:"replicas,omitempty"`

    // Node Selector to add to the pod
    // +optional
    NodeSelector map[string]string `json:"nodeSelector,omitempty"`

    // Affinity to add to the pod
    // +optional
    Affinity corev1.Affinity `json:"affinity,omitempty"`

	// Max Message Size to poll from SQS
	// +optional
	MaxBatchSize string `json:"maxBatchSize,omitempty"`

	// SendBatchedResponse will send all messages recieved as a single HTTP event rather than individual event per message
	// +optional
	SendBatchedResponse string `json:"sendBatchedResponse,omitempty"`

	// OnFailedPollWaitSecs sets the time SQSSource sleeps if it failed to poll from SQS
	// +optional
	OnFailedPollWaitSecs string `json:"onFailedPollWaitSecs,omitempty"`

	// WaitTimeSeconds Controls the maximum time to wait in the poll performed with
	// ReceiveMessageWithContext.  If there are no messages in the
	// given secs, the call times out and returns control to us.
	// +optional
	WaitTimeSeconds string `json:"waitTimeSeconds,omitempty"`

	// Sink is a reference to an object that will resolve to a domain name to
	// use as the sink.  This is where events will be received.
	// +optional
	Sink *corev1.ObjectReference `json:"sink,omitempty"` // TODO this is not the source duck anymore

	// ServiceAccoutName is the name of the ServiceAccount that will be used to
	// run the Receive Adapter Deployment.
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

const (
	// AwsSqsSourceEventType is the AWS SQS CloudEvent type.
	AwsSqsSourceEventType = "aws.sqs.message"
)

const (
	// AwsSqsSourceConditionReady has status True when the source is
	// ready to send events.
	AwsSqsSourceConditionReady = apis.ConditionReady

	// AwsSqsSourceConditionSinkProvided has status True when the
	// AwsSqsSource has been configured with a sink target.
	AwsSqsSourceConditionSinkProvided apis.ConditionType = "SinkProvided"

	// AwsSqsSourceConditionDeployed has status True when the
	// AwsSqsSource has had it's receive adapter deployment created.
	AwsSqsSourceConditionDeployed apis.ConditionType = "Deployed"
)

var condSet = apis.NewLivingConditionSet(
	AwsSqsSourceConditionSinkProvided,
	AwsSqsSourceConditionDeployed)

// AwsSqsSourceStatus defines the observed state of the source.
type AwsSqsSourceStatus struct {
	// inherits duck/v1 SourceStatus, which currently provides:
	// * ObservedGeneration - the 'Generation' of the Service that was last
	//   processed by the controller.
	// * Conditions - the latest available observations of a resource's current
	//   state.
	// * SinkURI - the current active sink URI that has been configured for the
	//   Source.
	duckv1.SourceStatus `json:",inline"`
}

// GetConditionSet retrieves the condition set for this resource. Implements the KRShaped interface.
func (*AwsSqsSource) GetConditionSet() apis.ConditionSet {
	return condSet
}

// GetStatus retrieves the duck status for this resource. Implements the KRShaped interface.
func (a *AwsSqsSource) GetStatus() *duckv1.Status {
	return &a.Status.Status
}

// GetCondition returns the condition currently associated with the given type, or nil.
func (s *AwsSqsSourceStatus) GetCondition(t apis.ConditionType) *apis.Condition {
	return condSet.Manage(s).GetCondition(t)
}

// IsReady returns true if the resource is ready overall.
func (s *AwsSqsSourceStatus) IsReady() bool {
	return condSet.Manage(s).IsHappy()
}

// InitializeConditions sets relevant unset conditions to Unknown state.
func (s *AwsSqsSourceStatus) InitializeConditions() {
	condSet.Manage(s).InitializeConditions()
}

// GetGroupVersionKind returns GroupVersionKind for AwsSqsSource
func (s *AwsSqsSource) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("AwsSqsSource")
}

// MarkSink sets the condition that the source has a sink configured.
func (s *AwsSqsSourceStatus) MarkSink(uri string) {
	if len(uri) > 0 {
		if u, err := apis.ParseURL(uri); err != nil {
			s.SinkURI = nil
			condSet.Manage(s).MarkUnknown(AwsSqsSourceConditionSinkProvided,
				"SinkEmpty", "Sink resolving resulted in an error. %s", err.Error())
		} else {
			s.SinkURI = u
			condSet.Manage(s).MarkTrue(AwsSqsSourceConditionSinkProvided)
		}
	} else {
		s.SinkURI = nil
		condSet.Manage(s).MarkUnknown(AwsSqsSourceConditionSinkProvided,
			"SinkEmpty", "Sink has resolved to empty.")
	}
}

// MarkNoSink sets the condition that the source does not have a sink configured.
func (s *AwsSqsSourceStatus) MarkNoSink(reason, messageFormat string, messageA ...interface{}) {
	condSet.Manage(s).MarkFalse(AwsSqsSourceConditionSinkProvided, reason, messageFormat, messageA...)
}

// MarkDeployed sets the condition that the source has been deployed.
func (s *AwsSqsSourceStatus) MarkDeployed() {
	condSet.Manage(s).MarkTrue(AwsSqsSourceConditionDeployed)
}

// MarkDeploying sets the condition that the source is deploying.
func (s *AwsSqsSourceStatus) MarkDeploying(reason, messageFormat string, messageA ...interface{}) {
	condSet.Manage(s).MarkUnknown(AwsSqsSourceConditionDeployed, reason, messageFormat, messageA...)
}

// MarkNotDeployed sets the condition that the source has not been deployed.
func (s *AwsSqsSourceStatus) MarkNotDeployed(reason, messageFormat string, messageA ...interface{}) {
	condSet.Manage(s).MarkFalse(AwsSqsSourceConditionDeployed, reason, messageFormat, messageA...)
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AwsSqsSourceList contains a list of AwsSqsSource
type AwsSqsSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AwsSqsSource `json:"items"`
}
