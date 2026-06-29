package server

import (
	"context"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/mar-schmidt/Podium/internal/adapter"
	"github.com/mar-schmidt/Podium/internal/config"
	"github.com/mar-schmidt/Podium/internal/core"
	"github.com/mar-schmidt/Podium/internal/store"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func TestWebSocketSendTurn(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	paths := config.NewPaths(home)
	if _, err := config.Scaffold(paths); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	if err := os.WriteFile(paths.BaseAgents, []byte("base layer\n"), 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}
	db, err := store.Open(paths.DB)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	fake := adapter.NewFake()
	fake.Responses = []string{"assistant from ws"}
	coreSvc, err := core.New(core.Options{Paths: paths, Store: db, Adapter: fake})
	if err != nil {
		t.Fatalf("new core: %v", err)
	}
	if _, err := coreSvc.CreateAgent(ctx, core.CreateAgentRequest{Name: "webber", Provider: config.ProviderClaude}); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	srv := New(Options{Bind: "127.0.0.1", Port: 0, Core: coreSvc})
	ts := httptest.NewServer(srv.httpSrv.Handler)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	if err := wsjson.Write(ctx, conn, ClientMessage{
		Type:      "send_turn",
		RequestID: "req-1",
		AgentName: "webber",
		Message:   "hello",
	}); err != nil {
		t.Fatalf("write send_turn: %v", err)
	}

	var sawSession, sawAssistant, sawDone bool
	for i := 0; i < 10; i++ {
		var msg ServerMessage
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			t.Fatalf("read ws: %v", err)
		}
		switch msg.Type {
		case "session":
			sawSession = msg.Session != nil && msg.Session.Origin == store.OriginWeb
		case "assistant":
			sawAssistant = msg.Delta == "assistant from ws"
		case "done":
			sawDone = true
		}
		if sawSession && sawAssistant && sawDone {
			return
		}
	}
	t.Fatalf("missing expected events: session=%v assistant=%v done=%v", sawSession, sawAssistant, sawDone)
}
