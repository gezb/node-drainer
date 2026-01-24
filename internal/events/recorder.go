/*
Copyright The Kubernetes Authors.

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

package events

import (
	"fmt"
	"strings"
	"time"

	"github.com/patrickmn/go-cache"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

	gezbcoukalphav1 "github.com/gezb/node-drainer/api/v1"
)

const (
	EventReasonNodeNotFound            = "Node not found"
	EventReasonDrainBlockedByPods      = "Draining blocked by pod(s)"
	EventReasonWaitingForPodsToRestart = "Waiting for pods to restart"
)

type Event struct {
	InvolvedObject runtime.Object
	Type           string
	Reason         string
	Message        string
	DedupeValues   []string
}

func (e Event) dedupeKey() string {
	return fmt.Sprintf("%s-%s",
		strings.ToLower(e.Reason),
		strings.Join(e.DedupeValues, "-"),
	)
}

type Recorder interface {
	Publish(...Event)
}

type recorder struct {
	rec   record.EventRecorder
	cache *cache.Cache
}

const defaultDedupeTimeout = 2 * time.Minute

func NewRecorder(r record.EventRecorder) Recorder {
	return &recorder{
		rec:   r,
		cache: cache.New(defaultDedupeTimeout, 10*time.Second),
	}
}

func (r *recorder) CreateEvent(nodeDrain *gezbcoukalphav1.NodeDrain, reason string, message string) {
	r.Publish(Event{
		InvolvedObject: nodeDrain,
		Type:           corev1.EventTypeNormal,
		Reason:         reason,
		Message:        message,
		DedupeValues:   []string{string(nodeDrain.UID)},
	})

}

// Publish creates a Kubernetes event using the passed event struct
func (r *recorder) Publish(evts ...Event) {
	for _, evt := range evts {
		r.publishEvent(evt)
	}
}

func (r *recorder) publishEvent(evt Event) {
	// Override the timeout if one is set for an event
	timeout := defaultDedupeTimeout
	// Dedupe same events that involve the same object and are close together
	if len(evt.DedupeValues) > 0 && !r.shouldCreateEvent(evt.dedupeKey(), timeout) {
		return
	}
	// Publish the event
	r.rec.Event(evt.InvolvedObject, evt.Type, evt.Reason, evt.Message)
}

func (r *recorder) shouldCreateEvent(key string, timeout time.Duration) bool {
	if _, exists := r.cache.Get(key); exists {
		return false
	}
	r.cache.Set(key, nil, timeout)
	return true
}
