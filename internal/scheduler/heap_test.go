package scheduler

import (
	"container/heap"
	"testing"
	"time"

	"uptiq/internal/config"
)

func TestScheduleHeap_Basic(t *testing.T) {
	h := newScheduleHeap()

	if h.Len() != 0 {
		t.Errorf("new heap should be empty, got len %d", h.Len())
	}

	if h.Peek() != nil {
		t.Error("Peek on empty heap should return nil")
	}
}

func TestScheduleHeap_PushPop(t *testing.T) {
	h := newScheduleHeap()

	now := time.Now()
	items := []*scheduledItem{
		{service: config.Service{ID: "svc-3"}, nextRun: now.Add(3 * time.Second)},
		{service: config.Service{ID: "svc-1"}, nextRun: now.Add(1 * time.Second)},
		{service: config.Service{ID: "svc-2"}, nextRun: now.Add(2 * time.Second)},
	}

	// Push items
	for _, item := range items {
		heap.Push(h, item)
	}

	if h.Len() != 3 {
		t.Errorf("heap len = %d, want 3", h.Len())
	}

	// Pop should return in order (earliest first)
	expected := []string{"svc-1", "svc-2", "svc-3"}
	for i, expectedID := range expected {
		item := heap.Pop(h).(*scheduledItem)
		if item.service.ID != expectedID {
			t.Errorf("pop %d: got %s, want %s", i, item.service.ID, expectedID)
		}
	}

	if h.Len() != 0 {
		t.Errorf("heap should be empty after popping all, got len %d", h.Len())
	}
}

func TestScheduleHeap_Peek(t *testing.T) {
	h := newScheduleHeap()

	now := time.Now()
	heap.Push(h, &scheduledItem{
		service: config.Service{ID: "svc-2"},
		nextRun: now.Add(2 * time.Second),
	})
	heap.Push(h, &scheduledItem{
		service: config.Service{ID: "svc-1"},
		nextRun: now.Add(1 * time.Second),
	})

	// Peek should return earliest without removing
	item := h.Peek()
	if item.service.ID != "svc-1" {
		t.Errorf("Peek returned %s, want svc-1", item.service.ID)
	}

	// Len should be unchanged
	if h.Len() != 2 {
		t.Errorf("Peek should not remove item, len = %d", h.Len())
	}

	// Peek again should return same
	item2 := h.Peek()
	if item2.service.ID != "svc-1" {
		t.Error("repeated Peek returned different item")
	}
}

func TestScheduleHeap_Clear(t *testing.T) {
	h := newScheduleHeap()

	now := time.Now()
	for i := 0; i < 5; i++ {
		heap.Push(h, &scheduledItem{
			service: config.Service{ID: "svc"},
			nextRun: now.Add(time.Duration(i) * time.Second),
		})
	}

	if h.Len() != 5 {
		t.Errorf("heap len before clear = %d, want 5", h.Len())
	}

	h.clear()

	if h.Len() != 0 {
		t.Errorf("heap len after clear = %d, want 0", h.Len())
	}

	if h.Peek() != nil {
		t.Error("Peek after clear should return nil")
	}
}

func TestScheduleHeap_Ordering(t *testing.T) {
	h := newScheduleHeap()

	// Add items with various times
	base := time.Now()
	times := []time.Duration{
		5 * time.Second,
		1 * time.Second,
		10 * time.Second,
		3 * time.Second,
		7 * time.Second,
	}

	for i, d := range times {
		heap.Push(h, &scheduledItem{
			service: config.Service{ID: "svc"},
			nextRun: base.Add(d),
		})
		if h.Len() != i+1 {
			t.Errorf("after push %d: len = %d, want %d", i, h.Len(), i+1)
		}
	}

	// Verify items come out in sorted order
	var lastTime time.Time
	for h.Len() > 0 {
		item := heap.Pop(h).(*scheduledItem)
		if !lastTime.IsZero() && item.nextRun.Before(lastTime) {
			t.Errorf("items not in order: got %v after %v", item.nextRun, lastTime)
		}
		lastTime = item.nextRun
	}
}

func TestScheduleHeap_IndexTracking(t *testing.T) {
	h := newScheduleHeap()

	now := time.Now()
	item1 := &scheduledItem{service: config.Service{ID: "svc-1"}, nextRun: now.Add(1 * time.Second)}
	item2 := &scheduledItem{service: config.Service{ID: "svc-2"}, nextRun: now.Add(2 * time.Second)}
	item3 := &scheduledItem{service: config.Service{ID: "svc-3"}, nextRun: now.Add(3 * time.Second)}

	heap.Push(h, item1)
	heap.Push(h, item2)
	heap.Push(h, item3)

	// All items should have valid indices
	for i, item := range *h {
		if item.index != i {
			t.Errorf("item %d: index = %d, want %d", i, item.index, i)
		}
	}
}

func TestScheduleHeap_LessFunction(t *testing.T) {
	now := time.Now()
	h := scheduleHeap{
		{nextRun: now.Add(1 * time.Second), index: 0},
		{nextRun: now.Add(2 * time.Second), index: 1},
	}

	// Earlier time should be "less"
	if !h.Less(0, 1) {
		t.Error("earlier item should be less than later item")
	}
	if h.Less(1, 0) {
		t.Error("later item should not be less than earlier item")
	}
}

func TestScheduleHeap_SwapFunction(t *testing.T) {
	now := time.Now()
	item1 := &scheduledItem{
		service: config.Service{ID: "svc-1"},
		nextRun: now.Add(1 * time.Second),
		index:   0,
	}
	item2 := &scheduledItem{
		service: config.Service{ID: "svc-2"},
		nextRun: now.Add(2 * time.Second),
		index:   1,
	}

	h := scheduleHeap{item1, item2}

	h.Swap(0, 1)

	// Items should be swapped
	if h[0].service.ID != "svc-2" {
		t.Error("swap failed: h[0] should be svc-2")
	}
	if h[1].service.ID != "svc-1" {
		t.Error("swap failed: h[1] should be svc-1")
	}

	// Indices should be updated
	if h[0].index != 0 {
		t.Errorf("after swap: h[0].index = %d, want 0", h[0].index)
	}
	if h[1].index != 1 {
		t.Errorf("after swap: h[1].index = %d, want 1", h[1].index)
	}
}

func TestScheduledItem_Fields(t *testing.T) {
	now := time.Now()
	item := scheduledItem{
		service: config.Service{
			ID:   "test-svc",
			Name: "Test Service",
			Type: "http",
		},
		nextRun: now,
		index:   5,
	}

	if item.service.ID != "test-svc" {
		t.Error("service ID mismatch")
	}
	if !item.nextRun.Equal(now) {
		t.Error("nextRun mismatch")
	}
	if item.index != 5 {
		t.Errorf("index = %d, want 5", item.index)
	}
}

func TestScheduleHeap_SameTime(t *testing.T) {
	h := newScheduleHeap()

	now := time.Now()
	// All items have same time
	for i := 0; i < 5; i++ {
		heap.Push(h, &scheduledItem{
			service: config.Service{ID: "svc"},
			nextRun: now,
		})
	}

	if h.Len() != 5 {
		t.Errorf("heap len = %d, want 5", h.Len())
	}

	// Should be able to pop all without error
	for h.Len() > 0 {
		item := heap.Pop(h).(*scheduledItem)
		if !item.nextRun.Equal(now) {
			t.Error("item time mismatch")
		}
	}
}

func TestScheduleHeap_Reschedule(t *testing.T) {
	h := newScheduleHeap()

	now := time.Now()
	item := &scheduledItem{
		service: config.Service{ID: "svc"},
		nextRun: now.Add(1 * time.Second),
	}

	heap.Push(h, item)

	// Pop item
	popped := heap.Pop(h).(*scheduledItem)

	// Reschedule with new time
	popped.nextRun = now.Add(10 * time.Second)
	heap.Push(h, popped)

	// Verify it's back in heap
	if h.Len() != 1 {
		t.Errorf("heap len after reschedule = %d, want 1", h.Len())
	}

	peeked := h.Peek()
	if !peeked.nextRun.Equal(now.Add(10 * time.Second)) {
		t.Error("rescheduled time not updated")
	}
}
