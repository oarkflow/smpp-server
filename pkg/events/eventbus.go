package events

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/oarkflow/smpp-server/pkg/smpp"
)

// EventBus implements a thread-safe event bus with pub/sub pattern
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[smpp.EventType][]smpp.EventHandler
	logger      smpp.Logger
	bufferSize  int
	async       bool
}

// NewEventBus creates a new event bus
func NewEventBus(logger smpp.Logger, bufferSize int, async bool) *EventBus {
	return &EventBus{
		subscribers: make(map[smpp.EventType][]smpp.EventHandler),
		logger:      logger,
		bufferSize:  bufferSize,
		async:       async,
	}
}

// Subscribe subscribes to events of a specific type
func (eb *EventBus) Subscribe(ctx context.Context, eventType smpp.EventType, handler smpp.EventHandler) error {
	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}

	eb.mu.Lock()
	defer eb.mu.Unlock()

	// Check if handler is already subscribed
	for _, h := range eb.subscribers[eventType] {
		if h.GetHandlerID() == handler.GetHandlerID() {
			return fmt.Errorf("handler %s already subscribed to event type %s", handler.GetHandlerID(), eventType)
		}
	}

	eb.subscribers[eventType] = append(eb.subscribers[eventType], handler)

	if eb.logger != nil {
		eb.logger.Debug("Handler subscribed to event",
			"handler_id", handler.GetHandlerID(),
			"event_type", eventType)
	}

	return nil
}

// Unsubscribe unsubscribes from events
func (eb *EventBus) Unsubscribe(ctx context.Context, eventType smpp.EventType, handler smpp.EventHandler) error {
	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}

	eb.mu.Lock()
	defer eb.mu.Unlock()

	handlers := eb.subscribers[eventType]
	for i, h := range handlers {
		if h.GetHandlerID() == handler.GetHandlerID() {
			// Remove handler from slice
			eb.subscribers[eventType] = append(handlers[:i], handlers[i+1:]...)

			if eb.logger != nil {
				eb.logger.Debug("Handler unsubscribed from event",
					"handler_id", handler.GetHandlerID(),
					"event_type", eventType)
			}

			return nil
		}
	}

	return fmt.Errorf("handler %s not found for event type %s", handler.GetHandlerID(), eventType)
}

// PublishSMSEvent publishes an SMS-related event
func (eb *EventBus) PublishSMSEvent(ctx context.Context, event *smpp.SMSEvent) error {
	return eb.publish(ctx, event)
}

// PublishConnectionEvent publishes a connection-related event
func (eb *EventBus) PublishConnectionEvent(ctx context.Context, event *smpp.ConnectionEvent) error {
	return eb.publish(ctx, event)
}

// PublishDeliveryEvent publishes a delivery report event
func (eb *EventBus) PublishDeliveryEvent(ctx context.Context, event *smpp.DeliveryEvent) error {
	return eb.publish(ctx, event)
}

// publish publishes an event to all subscribers
func (eb *EventBus) publish(ctx context.Context, event smpp.Event) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	eb.mu.RLock()
	handlers := make([]smpp.EventHandler, len(eb.subscribers[event.GetEventType()]))
	copy(handlers, eb.subscribers[event.GetEventType()])
	eb.mu.RUnlock()

	if len(handlers) == 0 {
		if eb.logger != nil {
			eb.logger.Debug("No handlers found for event type", "event_type", event.GetEventType())
		}
		return nil
	}

	if eb.async {
		// Handle events asynchronously
		for _, handler := range handlers {
			go eb.handleEventSafely(ctx, handler, event)
		}
	} else {
		// Handle events synchronously
		for _, handler := range handlers {
			if err := eb.handleEventSafely(ctx, handler, event); err != nil {
				if eb.logger != nil {
					eb.logger.Error("Error handling event",
						"handler_id", handler.GetHandlerID(),
						"event_type", event.GetEventType(),
						"error", err)
				}
			}
		}
	}

	return nil
}

// handleEventSafely handles an event with panic recovery
func (eb *EventBus) handleEventSafely(ctx context.Context, handler smpp.EventHandler, event smpp.Event) error {
	defer func() {
		if r := recover(); r != nil {
			if eb.logger != nil {
				eb.logger.Error("Panic in event handler",
					"handler_id", handler.GetHandlerID(),
					"event_type", event.GetEventType(),
					"panic", r)
			}
		}
	}()

	return handler.HandleEvent(ctx, event)
}

// GetSubscriberCount returns the number of subscribers for an event type
func (eb *EventBus) GetSubscriberCount(eventType smpp.EventType) int {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	return len(eb.subscribers[eventType])
}

// GetAllSubscriberCounts returns subscriber counts for all event types
func (eb *EventBus) GetAllSubscriberCounts() map[smpp.EventType]int {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	counts := make(map[smpp.EventType]int)
	for eventType, handlers := range eb.subscribers {
		counts[eventType] = len(handlers)
	}

	return counts
}

// Clear removes all subscribers
func (eb *EventBus) Clear() {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	eb.subscribers = make(map[smpp.EventType][]smpp.EventHandler)

	if eb.logger != nil {
		eb.logger.Debug("Event bus cleared")
	}
}

// AsyncEventBus implements an asynchronous event bus with buffered channels
type AsyncEventBus struct {
	*EventBus
	eventChan chan eventWrapper
	done      chan struct{}
	wg        sync.WaitGroup
}

type eventWrapper struct {
	ctx   context.Context
	event smpp.Event
}

// NewAsyncEventBus creates a new asynchronous event bus
func NewAsyncEventBus(logger smpp.Logger, bufferSize int) *AsyncEventBus {
	eb := &AsyncEventBus{
		EventBus:  NewEventBus(logger, bufferSize, false), // Set sync to false since we handle async here
		eventChan: make(chan eventWrapper, bufferSize),
		done:      make(chan struct{}),
	}

	eb.start()
	return eb
}

// start starts the async event processing
func (aeb *AsyncEventBus) start() {
	aeb.wg.Add(1)
	go func() {
		defer aeb.wg.Done()

		for {
			select {
			case eventWrap := <-aeb.eventChan:
				aeb.EventBus.publish(eventWrap.ctx, eventWrap.event)
			case <-aeb.done:
				return
			}
		}
	}()
}

// PublishSMSEvent publishes an SMS-related event asynchronously
func (aeb *AsyncEventBus) PublishSMSEvent(ctx context.Context, event *smpp.SMSEvent) error {
	return aeb.publishAsync(ctx, event)
}

// PublishConnectionEvent publishes a connection-related event asynchronously
func (aeb *AsyncEventBus) PublishConnectionEvent(ctx context.Context, event *smpp.ConnectionEvent) error {
	return aeb.publishAsync(ctx, event)
}

// PublishDeliveryEvent publishes a delivery report event asynchronously
func (aeb *AsyncEventBus) PublishDeliveryEvent(ctx context.Context, event *smpp.DeliveryEvent) error {
	return aeb.publishAsync(ctx, event)
}

// publishAsync publishes an event asynchronously
func (aeb *AsyncEventBus) publishAsync(ctx context.Context, event smpp.Event) error {
	select {
	case aeb.eventChan <- eventWrapper{ctx: ctx, event: event}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Channel is full, log warning and drop event
		if aeb.logger != nil {
			aeb.logger.Warn("Event bus channel full, dropping event",
				"event_type", event.GetEventType())
		}
		return fmt.Errorf("event bus channel full")
	}
}

// Close closes the async event bus
func (aeb *AsyncEventBus) Close() {
	close(aeb.done)
	aeb.wg.Wait()
	close(aeb.eventChan)
}

// EventHandlerFunc is a function type that implements EventHandler
type EventHandlerFunc struct {
	id      string
	handler func(ctx context.Context, event smpp.Event) error
}

// NewEventHandlerFunc creates a new EventHandlerFunc
func NewEventHandlerFunc(id string, handler func(ctx context.Context, event smpp.Event) error) *EventHandlerFunc {
	return &EventHandlerFunc{
		id:      id,
		handler: handler,
	}
}

// HandleEvent implements the EventHandler interface
func (ehf *EventHandlerFunc) HandleEvent(ctx context.Context, event smpp.Event) error {
	return ehf.handler(ctx, event)
}

// GetHandlerID implements the EventHandler interface
func (ehf *EventHandlerFunc) GetHandlerID() string {
	return ehf.id
}

// DefaultEventHandlers provides common event handlers

// LoggingEventHandler logs all events
type LoggingEventHandler struct {
	id     string
	logger smpp.Logger
}

// NewLoggingEventHandler creates a new logging event handler
func NewLoggingEventHandler(id string, logger smpp.Logger) *LoggingEventHandler {
	return &LoggingEventHandler{
		id:     id,
		logger: logger,
	}
}

// HandleEvent logs the event
func (leh *LoggingEventHandler) HandleEvent(ctx context.Context, event smpp.Event) error {
	if leh.logger != nil {
		leh.logger.Info("Event received",
			"event_type", event.GetEventType(),
			"timestamp", event.GetTimestamp(),
			"data", event.GetData())
	}
	return nil
}

// GetHandlerID returns the handler ID
func (leh *LoggingEventHandler) GetHandlerID() string {
	return leh.id
}

// MetricsEventHandler collects metrics for events
type MetricsEventHandler struct {
	id      string
	metrics smpp.MetricsCollector
}

// NewMetricsEventHandler creates a new metrics event handler
func NewMetricsEventHandler(id string, metrics smpp.MetricsCollector) *MetricsEventHandler {
	return &MetricsEventHandler{
		id:      id,
		metrics: metrics,
	}
}

// HandleEvent collects metrics for the event
func (meh *MetricsEventHandler) HandleEvent(ctx context.Context, event smpp.Event) error {
	if meh.metrics != nil {
		labels := map[string]string{
			"event_type": string(event.GetEventType()),
		}

		meh.metrics.IncCounter("events_total", labels)

		// Add specific metrics based on event type
		switch e := event.(type) {
		case *smpp.SMSEvent:
			if e.MessageID != "" {
				meh.metrics.IncCounter("sms_events_total", map[string]string{
					"message_id": e.MessageID,
				})
			}
		case *smpp.ConnectionEvent:
			if e.SessionID != "" {
				meh.metrics.IncCounter("connection_events_total", map[string]string{
					"session_id": e.SessionID,
				})
			}
		case *smpp.DeliveryEvent:
			if e.MessageID != "" {
				meh.metrics.IncCounter("delivery_events_total", map[string]string{
					"message_id": e.MessageID,
				})
			}
		}
	}

	return nil
}

// GetHandlerID returns the handler ID
func (meh *MetricsEventHandler) GetHandlerID() string {
	return meh.id
}

// EventBuilder helps building events
type EventBuilder struct{}

// NewEventBuilder creates a new event builder
func NewEventBuilder() *EventBuilder {
	return &EventBuilder{}
}

// BuildSMSEvent builds an SMS event
func (eb *EventBuilder) BuildSMSEvent(eventType smpp.EventType, messageID string, session *smpp.Session, message *smpp.Message) *smpp.SMSEvent {
	return &smpp.SMSEvent{
		Type:      eventType,
		Timestamp: time.Now(),
		MessageID: messageID,
		Session:   session,
		Message:   message,
		Data:      make(map[string]interface{}),
	}
}

// BuildConnectionEvent builds a connection event
func (eb *EventBuilder) BuildConnectionEvent(eventType smpp.EventType, sessionID string, session *smpp.Session, remoteAddr string) *smpp.ConnectionEvent {
	return &smpp.ConnectionEvent{
		Type:       eventType,
		Timestamp:  time.Now(),
		SessionID:  sessionID,
		Session:    session,
		RemoteAddr: remoteAddr,
		Data:       make(map[string]interface{}),
	}
}

// BuildDeliveryEvent builds a delivery event
func (eb *EventBuilder) BuildDeliveryEvent(eventType smpp.EventType, messageID string, report *smpp.DeliveryReport, session *smpp.Session) *smpp.DeliveryEvent {
	return &smpp.DeliveryEvent{
		Type:      eventType,
		Timestamp: time.Now(),
		MessageID: messageID,
		Report:    report,
		Session:   session,
		Data:      make(map[string]interface{}),
	}
}
