package notify_test

import (
	"strings"
	"testing"
	"time"

	"github.com/irchelper/agent-queue/internal/model"
	"github.com/irchelper/agent-queue/internal/notify"
)

// -------------------------------------------------------------------
// FormatDuration
// -------------------------------------------------------------------

func TestFormatDuration_NilStartedAt(t *testing.T) {
	got := notify.FormatDuration(nil, time.Now())
	if got != "未知" {
		t.Fatalf("expected 未知, got %q", got)
	}
}

func TestFormatDuration_LessThanOneMinute(t *testing.T) {
	start := time.Now().Add(-30 * time.Second)
	got := notify.FormatDuration(&start, time.Now())
	if got != "< 1 分钟" {
		t.Fatalf("expected '< 1 分钟', got %q", got)
	}
}

func TestFormatDuration_ExactlyOneMinute(t *testing.T) {
	// Use fixed reference time to avoid sub-millisecond drift.
	now := time.Unix(1000000, 0)
	start := now.Add(-60 * time.Second)
	got := notify.FormatDuration(&start, now)
	if got != "约 1 分钟" {
		t.Fatalf("expected '约 1 分钟', got %q", got)
	}
}

func TestFormatDuration_CeilingRounding(t *testing.T) {
	// 90 seconds → ceil(1.5) = 2 minutes
	now := time.Unix(1000000, 0)
	start := now.Add(-90 * time.Second)
	got := notify.FormatDuration(&start, now)
	if got != "约 2 分钟" {
		t.Fatalf("expected '约 2 分钟', got %q", got)
	}
}

func TestFormatDuration_ExactMultipleMinutes(t *testing.T) {
	now := time.Unix(1000000, 0)
	start := now.Add(-5 * time.Minute)
	got := notify.FormatDuration(&start, now)
	if got != "约 5 分钟" {
		t.Fatalf("expected '约 5 分钟', got %q", got)
	}
}

func TestFormatDuration_LargeValue(t *testing.T) {
	now := time.Unix(1000000, 0)
	start := now.Add(-70 * time.Minute)
	got := notify.FormatDuration(&start, now)
	if got != "约 70 分钟" {
		t.Fatalf("expected '约 70 分钟', got %q", got)
	}
}

// -------------------------------------------------------------------
// FormatMessage
// -------------------------------------------------------------------

func TestFormatMessage_WithUserID(t *testing.T) {
	now := time.Unix(2000000, 0)
	start := now.Add(-3 * time.Minute)
	task := model.Task{
		ID:         "abc123",
		Title:      "实现登录功能",
		AssignedTo: "coder",
		Result:     "PR #42 已合并",
		StartedAt:  &start,
	}
	msg := notify.FormatMessage(task, "987654321", now)

	assertContains(t, msg, "<@987654321>")
	assertContains(t, msg, "✅ 任务完成")
	assertContains(t, msg, "**任务：** 实现登录功能")
	assertContains(t, msg, "**专家：** coder")
	assertContains(t, msg, "约 3 分钟")
	assertContains(t, msg, "**结果：** PR #42 已合并")
	assertContains(t, msg, "`task_id: abc123`")
}

func TestFormatMessage_WithoutUserID(t *testing.T) {
	now := time.Unix(2000000, 0)
	task := model.Task{
		ID:    "xyz",
		Title: "测试任务",
	}
	msg := notify.FormatMessage(task, "", now)

	if strings.Contains(msg, "<@") {
		t.Fatalf("expected no mention when userID is empty, got: %s", msg)
	}
	assertContains(t, msg, "✅ 任务完成")
}

func TestFormatMessage_EmptyAssignedTo_ShowsUnknown(t *testing.T) {
	task := model.Task{ID: "1", Title: "t", AssignedTo: ""}
	msg := notify.FormatMessage(task, "", time.Now())
	assertContains(t, msg, "**专家：** 未知")
}

func TestFormatMessage_EmptyResult_ShowsNone(t *testing.T) {
	task := model.Task{ID: "1", Title: "t", Result: ""}
	msg := notify.FormatMessage(task, "", time.Now())
	assertContains(t, msg, "**结果：** （无）")
}

func TestFormatMessage_NilStartedAt_ShowsUnknownDuration(t *testing.T) {
	task := model.Task{ID: "1", Title: "t", StartedAt: nil}
	msg := notify.FormatMessage(task, "", time.Now())
	assertContains(t, msg, "**耗时：** 未知")
}

func TestFormatMessage_LessThanOneMinute(t *testing.T) {
	now := time.Unix(2000000, 0)
	start := now.Add(-10 * time.Second)
	task := model.Task{ID: "1", Title: "t", StartedAt: &start}
	msg := notify.FormatMessage(task, "", now)
	assertContains(t, msg, "**耗时：** < 1 分钟")
}

// -------------------------------------------------------------------
// helper
// -------------------------------------------------------------------

func assertContains(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("expected message to contain %q\nfull message:\n%s", sub, s)
	}
}
