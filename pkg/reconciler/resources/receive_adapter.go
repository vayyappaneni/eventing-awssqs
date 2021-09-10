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

package resources

import (
	"fmt"

	"knative.dev/pkg/kmeta"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/eventing-awssqs/pkg/apis/sources/v1alpha1"
)

// ReceiveAdapterArgs are the arguments needed to create an AWS SQS
// Source Receive Adapter. Every field is required.
type ReceiveAdapterArgs struct {
	Image   string
	Source  *v1alpha1.AwsSqsSource
	Labels  map[string]string
	SinkURI string
}

const (
	credsVolume    = "aws-credentials"
	credsMountPath = "/var/secrets/aws"
)

// MakeReceiveAdapter generates (but does not insert into K8s) the
// Receive Adapter Deployment for AWS SQS Sources.
func MakeReceiveAdapter(args *ReceiveAdapterArgs) *v1.Deployment {
	credsFile := ""
	if args.Source.Spec.AwsCredsSecret != nil {
		credsFile = fmt.Sprintf("%s/%s", credsMountPath, args.Source.Spec.AwsCredsSecret.Key)
	}

	annotations := map[string]string{"sidecar.istio.io/inject": "true"}
	for k, v := range args.Source.Spec.Annotations {
		annotations[k] = v
	}

	nodeSelectorMap := map[string]string{}
	for k, v := range args.Source.Spec.NodeSelector {
		nodeSelectorMap[k] = v
	}

	nodeAffinityMap := args.Source.Spec.Affinity

	envVars := []corev1.EnvVar{
		{
			Name:  "AWS_SQS_URL",
			Value: args.Source.Spec.QueueURL,
		},
		{
			Name:  "K_SINK",
			Value: args.SinkURI,
		},
	}
	maxBatchSizeProvided := args.Source.Spec.MaxBatchSize
	if len(maxBatchSizeProvided) != 0 {
		envVars = append(envVars, corev1.EnvVar{Name: "AWS_SQS_MAX_BATCH_SIZE", Value: maxBatchSizeProvided})
	}

	sendBatchedResponse := args.Source.Spec.SendBatchedResponse
	if len(sendBatchedResponse) != 0 {
		envVars = append(envVars, corev1.EnvVar{Name: "AWS_SQS_SEND_BATCH_RESPONSE", Value: sendBatchedResponse})
	}

	onFailedPollWaitSecs := args.Source.Spec.OnFailedPollWaitSecs
	if len(onFailedPollWaitSecs) != 0 {
		envVars = append(envVars, corev1.EnvVar{Name: "AWS_SQS_POLL_FAILED_WAIT_TIME", Value: onFailedPollWaitSecs})
	}

	waitTimeSeconds := args.Source.Spec.WaitTimeSeconds
	if len(waitTimeSeconds) != 0 {
		envVars = append(envVars, corev1.EnvVar{Name: "AWS_SQS_WAIT_TIME_SECONDS", Value: waitTimeSeconds})
	}

	replicasProvided := args.Source.Spec.Replicas
	replicas := int32(replicasProvided)

	volMounts := []corev1.VolumeMount(nil)
	vols := []corev1.Volume(nil)

	if credsFile != "" {
		envVars = append(envVars, corev1.EnvVar{Name: "AWS_APPLICATION_CREDENTIALS", Value: credsFile})
		volMounts = []corev1.VolumeMount{
			{
				Name:      credsVolume,
				MountPath: credsMountPath,
			},
		}

		vols = []corev1.Volume{
			{
				Name: credsVolume,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: args.Source.Spec.AwsCredsSecret.Name,
					},
				},
			},
		}
	}

	return &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    args.Source.Namespace,
			GenerateName: fmt.Sprintf("awssqs-%s-", args.Source.Name),
			Labels:       args.Labels,
			OwnerReferences: []metav1.OwnerReference{
				*kmeta.NewControllerRef(args.Source),
			},
		},
		Spec: v1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: args.Labels,
			},
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotations,
					Labels:      args.Labels,
				},
				Spec: corev1.PodSpec{
					// TODO: Expose NodeSelector here so that we can leverage it per CR
					ServiceAccountName: args.Source.Spec.ServiceAccountName,
					NodeSelector: nodeSelectorMap,
					Affinity: &nodeAffinityMap,
					Containers: []corev1.Container{
						{
							Name:         		"receive-adapter",
							Image:        		args.Image,
							ImagePullPolicy: 	corev1.PullIfNotPresent,
							Env:         		envVars,
							VolumeMounts: 		volMounts,
						},
					},
					Volumes: vols,
				},
			},
		},
	}
}
