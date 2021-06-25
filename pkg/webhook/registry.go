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

import "fmt"

var (
	// webhookRegistry is a map of webhooks, keyed by their unique
	// identifier.
	webhooksRegistry map[string]*Webhook
)

// GetWebhooks returns the list of webhooks that were registred with
// RegisterWebhook function.
func GetWebhooks() []*Webhook {
	webhooks := make([]*Webhook, 0, len(webhooksRegistry))
	for _, wh := range webhooksRegistry {
		webhooks = append(webhooks, wh)
	}
	return webhooks
}

// RegisterWebhook registers a new webhook within the webhook registry.
// This function will return an error if it tries to register two webhooks
// with the same unique identifier.
func RegisterWebhook(w *Webhook) error {
	if webhooksRegistry == nil {
		webhooksRegistry = make(map[string]*Webhook)
	}

	_, ok := webhooksRegistry[w.UID()]
	if ok {
		return fmt.Errorf("webhook %s already registred", w.UID())
	}
	webhooksRegistry[w.UID()] = w
	return nil
}
