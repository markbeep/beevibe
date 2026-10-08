package core

import (
	"encoding/json"
	"reflect"
	"testing"
)

// The wire types are the contract frozen in plans/api.md §2. These cases are the
// literal payloads from that document: a renamed, added, missing or mistyped
// field must fail here.

func assertJSON(t *testing.T, value any, want string) {
	t.Helper()
	got, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var gotMap, wantMap map[string]any
	if err := json.Unmarshal(got, &gotMap); err != nil {
		t.Fatalf("unmarshal produced: %v", err)
	}
	if err := json.Unmarshal([]byte(want), &wantMap); err != nil {
		t.Fatalf("unmarshal wanted: %v", err)
	}
	if !reflect.DeepEqual(gotMap, wantMap) {
		t.Fatalf("wire mismatch\n got: %s\nwant: %s", got, want)
	}
	// The key set must match exactly, so a dropped or extra field is caught even
	// when its value would be null on both sides.
	if len(gotMap) != len(wantMap) {
		t.Fatalf("field count mismatch: got %d, want %d (%s)", len(gotMap), len(wantMap), got)
	}
}

func TestRoomJSONWireFormat(t *testing.T) {
	assertJSON(t, RoomJSON{
		ID:          "AB12CD",
		Name:        new("Friday demo"),
		State:       RoomOpen,
		Model:       "deepseek-flash",
		UserCount:   12,
		HasTemplate: true,
		CreatedAt:   1760000000,
		StartedAt:   new(int64(1760000100)),
	}, `{ "id": "AB12CD", "name": "Friday demo", "state": "open",
	      "model": "deepseek-flash", "userCount": 12, "hasTemplate": true,
	      "createdAt": 1760000000, "startedAt": 1760000100,
	      "closedAt": null, "archivedAt": null }`)

	assertJSON(t, RoomJSON{
		ID:         "AB12CD",
		Name:       nil,
		State:      RoomArchived,
		Model:      "deepseek-chat",
		UserCount:  0,
		CreatedAt:  1760000000,
		StartedAt:  nil,
		ClosedAt:   new(int64(1760000200)),
		ArchivedAt: new(int64(1760000300)),
	}, `{ "id": "AB12CD", "name": null, "state": "archived",
	      "model": "deepseek-chat", "userCount": 0, "hasTemplate": false,
	      "createdAt": 1760000000, "startedAt": null,
	      "closedAt": 1760000200, "archivedAt": 1760000300 }`)
}

func TestUserJSONWireFormat(t *testing.T) {
	assertJSON(t, UserJSON{
		ID:           42,
		RoomID:       "AB12CD",
		Name:         "Alice",
		Token:        "a1B2c3D4",
		CreatedAt:    1760000000,
		KickedAt:     nil,
		TokenLimit:   nil,
		TokensUsed:   12345,
		Online:       true,
		MicOn:        false,
		HelpPending:  false,
		AgentState:   AgentIdle,
		QueueDepth:   0,
		PreviewState: PreviewOK,
	}, `{ "id": 42, "roomId": "AB12CD", "name": "Alice", "token": "a1B2c3D4",
	      "createdAt": 1760000000, "kickedAt": null,
	      "tokenLimit": null, "tokensUsed": 12345,
	      "online": true, "micOn": false, "helpPending": false,
	      "agentState": "idle", "queueDepth": 0, "previewState": "ok" }`)

	assertJSON(t, UserJSON{
		ID:           7,
		RoomID:       "AB12CD",
		Name:         "Bob",
		Token:        "z9Y8x7W6",
		CreatedAt:    1760000001,
		KickedAt:     new(int64(1760000500)),
		TokenLimit:   new(int64(50000)),
		TokensUsed:   50000,
		Online:       false,
		MicOn:        true,
		HelpPending:  true,
		AgentState:   AgentEditing,
		QueueDepth:   2,
		PreviewState: PreviewFailed,
	}, `{ "id": 7, "roomId": "AB12CD", "name": "Bob", "token": "z9Y8x7W6",
	      "createdAt": 1760000001, "kickedAt": 1760000500,
	      "tokenLimit": 50000, "tokensUsed": 50000,
	      "online": false, "micOn": true, "helpPending": true,
	      "agentState": "editing", "queueDepth": 2, "previewState": "failed" }`)
}

func TestMessageJSONWireFormat(t *testing.T) {
	assertJSON(t, MessageJSON{
		ID:   99,
		Kind: KindTranscript,
		Text: "make the heading bigger",
		At:   1760000123,
	}, `{ "id": 99, "kind": "transcript", "text": "make the heading bigger", "at": 1760000123 }`)
}

func TestStatsJSONWireFormat(t *testing.T) {
	assertJSON(t, StatsJSON{
		TokensUsed:           12345,
		TokenLimit:           nil,
		LOC:                  210,
		SitePath:             "/rooms/AB12CD/42/",
		AgentContextMessages: 14,
	}, `{ "tokensUsed": 12345, "tokenLimit": null, "loc": 210,
	      "sitePath": "/rooms/AB12CD/42/", "agentContextMessages": 14 }`)
}

func TestAgentStateFramesAreFrozen(t *testing.T) {
	assertJSON(t, frameAgentState{
		Type:       "agent.state",
		State:      AgentIdle,
		QueueDepth: 0,
		RunID:      nil,
		Blocked:    false,
	}, `{ "type": "agent.state", "state": "idle", "queueDepth": 0, "runId": null, "blocked": false }`)

	assertJSON(t, frameAgentState{
		Type:       "agent.state",
		State:      AgentThinking,
		QueueDepth: 3,
		RunID:      new(int64(7)),
		Blocked:    true,
	}, `{ "type": "agent.state", "state": "thinking", "queueDepth": 3, "runId": 7, "blocked": true }`)

	assertJSON(t, frameChatAppend{
		Type:    "chat.append",
		Message: MessageJSON{ID: 99, Kind: KindSystem, Text: "hi", At: 1760000123},
	}, `{ "type": "chat.append", "message": { "id": 99, "kind": "system", "text": "hi", "at": 1760000123 } }`)

	assertJSON(t, frameSiteUpdated{Type: "site.updated", Rev: 12},
		`{ "type": "site.updated", "rev": 12 }`)

	assertJSON(t, frameRoomState{Type: "room.state", State: RoomClosed},
		`{ "type": "room.state", "state": "closed" }`)

	assertJSON(t, frameResetProgress{Type: "reset.progress", Scope: ResetScopeTemplate, Total: 5, Done: 2, Running: true},
		`{ "type": "reset.progress", "scope": "template", "total": 5, "done": 2, "running": true }`)
}

func TestHelloFrameWireFormat(t *testing.T) {
	roomID, userID, roomState := "AB12CD", int64(42), RoomStarted
	assertJSON(t,
		HelloFrame("user", &roomID, &userID, &roomState, AgentIdle, 0, false),
		`{ "type": "hello", "role": "user", "roomId": "AB12CD", "userId": 42,
		   "roomState": "started", "agentState": "idle", "queueDepth": 0, "blocked": false }`)

	assertJSON(t,
		HelloFrame("admin", nil, nil, nil, AgentIdle, 0, false),
		`{ "type": "hello", "role": "admin", "roomId": null, "userId": null,
		   "roomState": null, "agentState": "idle", "queueDepth": 0, "blocked": false }`)

	assertJSON(t, PongFrame(), `{ "type": "pong" }`)
}

func TestCannedMessagesAreVerbatim(t *testing.T) {
	want := map[string]string{
		MsgUnintelligible: "I didn't catch that — please say it again.",
		MsgCancelled:      "Prompt cancelled.",
		MsgTimedOut:       "The agent timed out — please try again.",
		MsgAgentError:     "Something went wrong — please try again.",
		MsgTokenLimit:     "Token limit reached — asking the admin to raise it.",
		MsgRoomNotStarted: "The room hasn't started yet.",
		MsgRoomClosed:     "The room is closed — editing is disabled.",
		MsgResetStart:     "Your site is being reset…",
		MsgResetDone:      "Your site has been reset.",
		MsgSTTFailed:      "Couldn't transcribe that — please try again.",
		MsgNewSession:     "New agent session — the agent forgot the earlier conversation.",
		MsgHelpRequest:    "Needs help.",
	}
	for got, expect := range want {
		if got != expect {
			t.Errorf("canned message mismatch:\n got: %q\nwant: %q", got, expect)
		}
	}
}
