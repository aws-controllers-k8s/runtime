// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package webhook

import (
	"fmt"

	ctrlrt "sigs.k8s.io/controller-runtime"
)

type WebhookType string

const (
	WebhookTypeUnknown    WebhookType = "unknown"
	WebhookTypeConversion WebhookType = "conversion"
	//TODO(a-hilaly) add validating and defaulting types
)

// Webhook contains information about a custom Webhook
type Webhook struct {
	Type       string
	CRDKind    string
	APIVersion string
	// Setup is used to register the webhook within
	// the webhook manager
	Setup func(ctrlrt.Manager) error
}

// UID returns a unique identifier for the webhook. Webhooks
// must be unique per CRD and APIVersion.
func (w *Webhook) UID() string {
	return fmt.Sprintf("%s/%s/%s", w.Type, w.CRDKind, w.APIVersion)
}

// New instanciate a new webhook object pointer.
func New(
	apiVersion string,
	crdKind string,
	type_ string,
	setupFunc func(ctrlrt.Manager) error,
) *Webhook {
	return &Webhook{
		Type:       type_,
		CRDKind:    crdKind,
		APIVersion: apiVersion,
		Setup:      setupFunc,
	}
}
