package test

import (
	"encoding/json"
	"testing"
)

// Test reaccess event
func TestReaccessEvent(t *testing.T) {
	runTest(t, func(s *Session) {
		c := s.Connect()
		cid := subscribeToTestModel(t, s, c)

		// Send reaccess event
		s.ResourceEvent("test.model", "reaccess", nil)

		// Validate an access request is triggered
		s.GetRequest(t).
			AssertSubject(t, "access.test.model").
			AssertPathPayload(t, "cid", cid).
			RespondSuccess(json.RawMessage(`{"get":true}`))
		c.AssertNoEvent(t, "test.model")
	})
}

// Test that a reaccess event triggers a re-access call on subscribed resources
// and that the resource are still subscribed after given access
func TestReaccessEventTriggersAccessCallOnSubscribedResources(t *testing.T) {
	runTest(t, func(s *Session) {
		event := json.RawMessage(`{"foo":"bar"}`)

		c := s.Connect()

		// Get linked model
		subscribeToTestModelParent(t, s, c, false)

		// Change token
		s.ResourceEvent("test.model.parent", "reaccess", nil)

		// Handle access requests with access denied
		s.GetRequest(t).AssertSubject(t, "access.test.model.parent").RespondSuccess(json.RawMessage(`{"get":true}`))

		// Validate no unsubscribe events are sent to client
		c.AssertNoEvent(t, "test.model.parent")

		// Send event on model and validate client event
		s.ResourceEvent("test.model", "custom", event)
		c.GetEvent(t).Equals(t, "test.model.custom", event)

		// Send event on model parent and validate client event
		s.ResourceEvent("test.model.parent", "custom", event)
		c.GetEvent(t).Equals(t, "test.model.parent.custom", event)
	})
}

// Test that a reaccess event triggers a re-access call on subscribed resources
// and that the resource are unsubscribed after being deniedn access
func TestReaccessEventTriggersUnsubscribeOnDeniedAccessCall(t *testing.T) {
	runTest(t, func(s *Session) {
		event := json.RawMessage(`{"foo":"bar"}`)
		reasonAccessDenied := json.RawMessage(`{"reason":{"code":"system.accessDenied","message":"Access denied"}}`)

		c := s.Connect()

		// Get linked model
		subscribeToTestModelParent(t, s, c, false)

		// Change token
		s.ResourceEvent("test.model.parent", "reaccess", nil)

		// Handle access requests with access denied
		s.GetRequest(t).AssertSubject(t, "access.test.model.parent").RespondSuccess(json.RawMessage(`{"get":false}`))

		// Validate unsubscribe events are sent to client
		c.GetEvent(t).AssertEventName(t, "test.model.parent.unsubscribe").AssertData(t, reasonAccessDenied)

		// Send event on model and validate client event
		s.ResourceEvent("test.model", "custom", event)
		c.AssertNoEvent(t, "test.model")

		// Send event on model parent and validate client event
		s.ResourceEvent("test.model.parent", "custom", event)
		c.AssertNoEvent(t, "test.model.parent")
	})
}
