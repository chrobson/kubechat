package main

import (
    "context"
    "testing"

    chatpb "kubechat/proto/chat"
)

// --- Given/When/Then style test for history ---
func TestChat_GetMessageHistory_EmptyWhenNoStore(t *testing.T) {
    t.Run("empty history without store", func(t *testing.T) {
        // --- Given ---
        svc := &server{natsConn: nil, messageStoreConn: nil}

        // --- When ---
        resp, err := svc.GetMessageHistory(context.Background(), &chatpb.GetMessageHistoryRequest{
            UserId1: "u1",
            UserId2: "u2",
            Limit:   10,
            Offset:  0,
        })

        // --- Then ---
        if err != nil { t.Fatalf("unexpected error: %v", err) }
        if len(resp.GetMessages()) != 0 { t.Fatalf("expected empty history") }
    })
}


