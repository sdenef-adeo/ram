// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the 'License');
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an 'AS IS' BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gcf

import (
	"context"
	"fmt"
	"log"
	"time"

	"cloud.google.com/go/functions/metadata"
)

// IntialRetryCheck performs intitial controls
// 1) return true and metadata when controls are passed
// 2) return false when controls failed:
// - 2a) with an error to retry the cloud function entry point function
// - 2b) with nil to stop the cloud function entry point function
func IntialRetryCheck(ctxEvent context.Context, initFailed bool, retryTimeOutSeconds int64) (bool, *metadata.Metadata, error) {
	metadata, err := metadata.FromContext(ctxEvent)
	if err != nil {
		// Assume an error on the function invoker and try again.
		return false, metadata, fmt.Errorf("metadata.FromContext: %v", err) // RETRY
	}
	if initFailed {
		log.Println("ERROR - init function failed")
		return false, metadata, nil // NO RETRY
	}

	// Ignore events that are too old.
	expiration := metadata.Timestamp.Add(time.Duration(retryTimeOutSeconds) * time.Second)
	if time.Now().After(expiration) {
		log.Printf("ERROR - too many retries for expired event '%q'", metadata.EventID)
		return false, metadata, nil // NO MORE RETRY
	}
	return true, metadata, nil
}