// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package walk

type sessionEndingEventHandlerInfo struct {
	handler SessionEndingEventHandler
	once    bool
}

// SessionEndingEventHandler is called with a pointer the handler sets to true
// to cancel the end of the session, and the reason.
type SessionEndingEventHandler func(canceled *bool, reason EndSessionReason)

// SessionEndingEvent is an event of the system service; see
// SystemService.SessionEnding.
type SessionEndingEvent struct {
	handlers []sessionEndingEventHandlerInfo
}

func (e *SessionEndingEvent) Attach(handler SessionEndingEventHandler) int {
	handlerInfo := sessionEndingEventHandlerInfo{handler, false}

	for i, h := range e.handlers {
		if h.handler == nil {
			e.handlers[i] = handlerInfo
			return i
		}
	}

	e.handlers = append(e.handlers, handlerInfo)

	return len(e.handlers) - 1
}

func (e *SessionEndingEvent) Detach(handle int) {
	e.handlers[handle].handler = nil
}

func (e *SessionEndingEvent) Once(handler SessionEndingEventHandler) {
	i := e.Attach(handler)
	e.handlers[i].once = true
}

// SessionEndingEventPublisher publishes a SessionEndingEvent.
type SessionEndingEventPublisher struct {
	event SessionEndingEvent
}

func (p *SessionEndingEventPublisher) Event() *SessionEndingEvent {
	return &p.event
}

func (p *SessionEndingEventPublisher) Publish(canceled *bool, reason EndSessionReason) {
	for i, h := range p.event.handlers {
		if h.handler != nil {
			h.handler(canceled, reason)

			if h.once {
				p.event.Detach(i)
			}
		}
	}
}

type sessionEndedEventHandlerInfo struct {
	handler SessionEndedEventHandler
	once    bool
}

// SessionEndedEventHandler is called with whether the session really ends and
// the reason.
type SessionEndedEventHandler func(ending bool, reason EndSessionReason)

// SessionEndedEvent is an event of the system service; see
// SystemService.SessionEnded.
type SessionEndedEvent struct {
	handlers []sessionEndedEventHandlerInfo
}

func (e *SessionEndedEvent) Attach(handler SessionEndedEventHandler) int {
	handlerInfo := sessionEndedEventHandlerInfo{handler, false}

	for i, h := range e.handlers {
		if h.handler == nil {
			e.handlers[i] = handlerInfo
			return i
		}
	}

	e.handlers = append(e.handlers, handlerInfo)

	return len(e.handlers) - 1
}

func (e *SessionEndedEvent) Detach(handle int) {
	e.handlers[handle].handler = nil
}

func (e *SessionEndedEvent) Once(handler SessionEndedEventHandler) {
	i := e.Attach(handler)
	e.handlers[i].once = true
}

// SessionEndedEventPublisher publishes a SessionEndedEvent.
type SessionEndedEventPublisher struct {
	event SessionEndedEvent
}

func (p *SessionEndedEventPublisher) Event() *SessionEndedEvent {
	return &p.event
}

func (p *SessionEndedEventPublisher) Publish(ending bool, reason EndSessionReason) {
	for i, h := range p.event.handlers {
		if h.handler != nil {
			h.handler(ending, reason)

			if h.once {
				p.event.Detach(i)
			}
		}
	}
}

type sessionChangedEventHandlerInfo struct {
	handler SessionChangedEventHandler
	once    bool
}

// SessionChangedEventHandler is called with the change and the ID of the
// session it applies to.
type SessionChangedEventHandler func(change SessionChange, sessionID uint32)

// SessionChangedEvent is an event of the system service; see
// SystemService.SessionChanged.
type SessionChangedEvent struct {
	handlers []sessionChangedEventHandlerInfo
}

func (e *SessionChangedEvent) Attach(handler SessionChangedEventHandler) int {
	handlerInfo := sessionChangedEventHandlerInfo{handler, false}

	for i, h := range e.handlers {
		if h.handler == nil {
			e.handlers[i] = handlerInfo
			return i
		}
	}

	e.handlers = append(e.handlers, handlerInfo)

	return len(e.handlers) - 1
}

func (e *SessionChangedEvent) Detach(handle int) {
	e.handlers[handle].handler = nil
}

func (e *SessionChangedEvent) Once(handler SessionChangedEventHandler) {
	i := e.Attach(handler)
	e.handlers[i].once = true
}

// SessionChangedEventPublisher publishes a SessionChangedEvent.
type SessionChangedEventPublisher struct {
	event SessionChangedEvent
}

func (p *SessionChangedEventPublisher) Event() *SessionChangedEvent {
	return &p.event
}

func (p *SessionChangedEventPublisher) Publish(change SessionChange, sessionID uint32) {
	for i, h := range p.event.handlers {
		if h.handler != nil {
			h.handler(change, sessionID)

			if h.once {
				p.event.Detach(i)
			}
		}
	}
}

type settingsChangedEventHandlerInfo struct {
	handler SettingsChangedEventHandler
	once    bool
}

// SettingsChangedEventHandler is called with the changed settings.
type SettingsChangedEventHandler func(change SettingsChange)

// SettingsChangedEvent is an event of the system service; see
// SystemService.SettingsChanged.
type SettingsChangedEvent struct {
	handlers []settingsChangedEventHandlerInfo
}

func (e *SettingsChangedEvent) Attach(handler SettingsChangedEventHandler) int {
	handlerInfo := settingsChangedEventHandlerInfo{handler, false}

	for i, h := range e.handlers {
		if h.handler == nil {
			e.handlers[i] = handlerInfo
			return i
		}
	}

	e.handlers = append(e.handlers, handlerInfo)

	return len(e.handlers) - 1
}

func (e *SettingsChangedEvent) Detach(handle int) {
	e.handlers[handle].handler = nil
}

func (e *SettingsChangedEvent) Once(handler SettingsChangedEventHandler) {
	i := e.Attach(handler)
	e.handlers[i].once = true
}

// SettingsChangedEventPublisher publishes a SettingsChangedEvent.
type SettingsChangedEventPublisher struct {
	event SettingsChangedEvent
}

func (p *SettingsChangedEventPublisher) Event() *SettingsChangedEvent {
	return &p.event
}

func (p *SettingsChangedEventPublisher) Publish(change SettingsChange) {
	for i, h := range p.event.handlers {
		if h.handler != nil {
			h.handler(change)

			if h.once {
				p.event.Detach(i)
			}
		}
	}
}
