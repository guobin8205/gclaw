package tui

import "testing"

func TestApprovalYes(t *testing.T) {
	a := ApprovalRequest{}
	if a.HandleKey("y") != ApprovalAllow {
		t.Error("expected Allow")
	}
}

func TestApprovalNo(t *testing.T) {
	a := ApprovalRequest{}
	if a.HandleKey("n") != ApprovalDeny {
		t.Error("expected Deny")
	}
}

func TestApprovalAlways(t *testing.T) {
	a := ApprovalRequest{}
	if a.HandleKey("a") != ApprovalAlways {
		t.Error("expected Always")
	}
}

func TestApprovalEsc(t *testing.T) {
	a := ApprovalRequest{}
	if a.HandleKey("esc") != ApprovalCancel {
		t.Error("expected Cancel")
	}
}

func TestApprovalPending(t *testing.T) {
	a := ApprovalRequest{}
	if a.HandleKey("x") != ApprovalPending {
		t.Error("expected Pending")
	}
}

func TestApprovalRender(t *testing.T) {
	a := ApprovalRequest{ToolName: "Bash", Detail: "rm -rf /tmp"}
	if a.Render(LoadTheme("tokyo-night")) == "" {
		t.Error("expected non-empty")
	}
}
