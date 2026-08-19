package engine

import (
	"context"
	"testing"
)

func TestApprovalWaterfallFirstAnswerWins(t *testing.T) {
	w := NewApprovalWaterfall()
	first := false
	w.Add(func(ctx context.Context, req ApprovalRequest) (ApprovalResponse, bool) {
		first = true
		return ApprovalApprove, true
	})
	second := false
	w.Add(func(ctx context.Context, req ApprovalRequest) (ApprovalResponse, bool) {
		second = true
		return ApprovalReject, true
	})

	resp, msg := w.Decide(context.Background(), ApprovalRequest{ToolName: "Bash", Category: ApprovalFileDeletion})
	if resp != ApprovalApprove {
		t.Fatalf("resp = %v, want Approve", resp)
	}
	if first == false || second {
		t.Fatalf("decider order wrong: first=%v second=%v", first, second)
	}
	if msg != "" {
		t.Fatalf("unexpected deny message %q", msg)
	}
}

func TestApprovalWaterfallAbstainsToNext(t *testing.T) {
	w := NewApprovalWaterfall()
	w.Add(func(ctx context.Context, req ApprovalRequest) (ApprovalResponse, bool) {
		return ApprovalReject, false // abstain
	})
	w.Add(func(ctx context.Context, req ApprovalRequest) (ApprovalResponse, bool) {
		return ApprovalApproveForSession, true
	})

	resp, _ := w.Decide(context.Background(), ApprovalRequest{Category: ApprovalNetwork})
	if resp != ApprovalApproveForSession {
		t.Fatalf("resp = %v, want ApproveForSession", resp)
	}
}

func TestApprovalWaterfallFailClosedNoAnswerer(t *testing.T) {
	w := NewApprovalWaterfall()
	resp, msg := w.Decide(context.Background(), ApprovalRequest{ToolName: "Bash", Category: ApprovalFileDeletion})
	if resp != ApprovalReject {
		t.Fatalf("resp = %v, want Reject", resp)
	}
	if msg == "" {
		t.Fatal("fail-closed must explain the denial")
	}
}

func TestApprovalWaterfallDisposer(t *testing.T) {
	w := NewApprovalWaterfall()
	answered := false
	dispose := w.Add(func(ctx context.Context, req ApprovalRequest) (ApprovalResponse, bool) {
		answered = true
		return ApprovalApprove, true
	})
	dispose()
	resp, _ := w.Decide(context.Background(), ApprovalRequest{Category: ApprovalNetwork})
	if resp != ApprovalReject {
		t.Fatalf("resp = %v, want Reject after disposer removed the only decider", resp)
	}
	if answered {
		t.Fatal("disposed decider ran")
	}
}
