package events_test

import (
	"sync"
	"testing"

	gezbcoukalphav1 "github.com/gezb/node-drainer/api/v1"
	"github.com/gezb/node-drainer/internal/events"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
)

var eventRecorder events.Recorder
var internalRecorder *InternalRecorder

type InternalRecorder struct {
	mu    sync.RWMutex
	calls map[string]int
}

func NewInternalRecorder() *InternalRecorder {
	return &InternalRecorder{
		calls: map[string]int{},
	}
}

func (i *InternalRecorder) Event(_ runtime.Object, _, reason, _ string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.calls[reason]++
}

func (i *InternalRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, _ ...interface{}) {
	i.Event(object, eventtype, reason, messageFmt)
}

func (i *InternalRecorder) AnnotatedEventf(object runtime.Object, _ map[string]string, eventtype, reason, messageFmt string, _ ...interface{}) {
	i.Event(object, eventtype, reason, messageFmt)
}

func (i *InternalRecorder) Calls(reason string) int {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.calls[reason]
}

func TestRecorder(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "EventRecorder")
}

var testEvent = events.Event{
	InvolvedObject: nodeDrainWithUID(),
	Type:           corev1.EventTypeNormal,
	Reason:         "reason for event",
	Message:        "message for event",
	DedupeValues:   []string{string(nodeDrainWithUID().UID)},
}

var _ = BeforeEach(func() {
	internalRecorder = NewInternalRecorder()
	eventRecorder = events.NewRecorder(internalRecorder)

})

var _ = Describe("Event Creation", func() {
	It("should create a NodeDrain event", func() {
		eventRecorder.Publish(testEvent)

		Expect(internalRecorder.Calls(testEvent.Reason)).To(Equal(1))
	})

})

var _ = Describe("Dedupe", func() {
	It("should only create a single event when many events are created quickly", func() {
		for i := 0; i < 100; i++ {
			eventRecorder.Publish(testEvent)
		}
		Expect(internalRecorder.Calls(testEvent.Reason)).To(Equal(1))
	})
})

func nodeDrainWithUID() *gezbcoukalphav1.NodeDrain {
	p := gezbcoukalphav1.NodeDrain{}
	p.UID = uuid.NewUUID()
	return &p
}
