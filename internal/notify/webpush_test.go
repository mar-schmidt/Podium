package notify

import (
	"encoding/json"
	"testing"
)

func TestWebPushPayloadIncludesApprovalAction(t *testing.T) {
	payload := webPushPayloadForNotification(Notification{
		SessionID: "session-1",
		Title:     "Agent needs approval",
		Body:      "A tool action is waiting for your decision.",
		Kind:      "permission",
		Approval: &ApprovalAction{
			RequestID: "perm-1",
			Input:     json.RawMessage(`{"command":"echo ok"}`),
		},
	})

	if payload.Approval == nil {
		t.Fatal("approval action was not included")
	}
	if payload.Approval.RequestID != "perm-1" {
		t.Fatalf("request id = %q, want perm-1", payload.Approval.RequestID)
	}
	if string(payload.Approval.Input) != `{"command":"echo ok"}` {
		t.Fatalf("approval input = %s", payload.Approval.Input)
	}
}

func TestWebPushPayloadOmitsApprovalActionForQuestions(t *testing.T) {
	payload := webPushPayloadForNotification(Notification{
		SessionID: "session-1",
		Title:     "Agent has a question",
		Kind:      "question",
	})

	if payload.Approval != nil {
		t.Fatalf("question payload should not include approval action: %+v", payload.Approval)
	}
}
