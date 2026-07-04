package lab

import (
	"context"
	"errors"
	"testing"
)

func TestCleanupStack_AddAndRun(t *testing.T) {
	stack := NewCleanupStack()
	ctx := context.Background()

	var order []string
	var mu bool

	stack.Add("first", func(ctx context.Context) error {
		if mu {
			t.Error("mutex not released before second call")
		}
		mu = true
		order = append(order, "first")
		mu = false
		return nil
	})
	stack.Add("second", func(ctx context.Context) error {
		if mu {
			t.Error("mutex not released before third call")
		}
		mu = true
		order = append(order, "second")
		mu = false
		return nil
	})
	stack.Add("third", func(ctx context.Context) error {
		if mu {
			t.Error("mutex not released before fourth call")
		}
		mu = true
		order = append(order, "third")
		mu = false
		return nil
	})

	errs := stack.Run(ctx)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}

	// Should run in reverse order
	if len(order) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(order))
	}
	if order[0] != "third" {
		t.Errorf("expected first to run third, got %s", order[0])
	}
	if order[1] != "second" {
		t.Errorf("expected second to run second, got %s", order[1])
	}
	if order[2] != "first" {
		t.Errorf("expected third to run first, got %s", order[2])
	}
}

func TestCleanupStack_ReportsErrors(t *testing.T) {
	stack := NewCleanupStack()
	ctx := context.Background()

	expectedErr := errors.New("cleanup failed")

	stack.Add("fail", func(ctx context.Context) error {
		return expectedErr
	})

	errs := stack.Run(ctx)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0] != expectedErr {
		t.Errorf("expected error '%v', got '%v'", expectedErr, errs[0])
	}
}

func TestCleanupStack_ContinuesAfterError(t *testing.T) {
	stack := NewCleanupStack()
	ctx := context.Background()

	var order []string

	stack.Add("first", func(ctx context.Context) error {
		order = append(order, "first")
		return nil
	})
	stack.Add("fail", func(ctx context.Context) error {
		order = append(order, "fail")
		return errors.New("expected error")
	})
	stack.Add("second", func(ctx context.Context) error {
		order = append(order, "second")
		return nil
	})

	errs := stack.Run(ctx)
	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d", len(errs))
	}

	// All steps should have run
	if len(order) != 3 {
		t.Errorf("expected 3 steps, got %d: %v", len(order), order)
	}
}

func TestCleanupStack_Empty(t *testing.T) {
	stack := NewCleanupStack()
	ctx := context.Background()

	errs := stack.Run(ctx)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestCleanupStack_CannotReuse(t *testing.T) {
	stack := NewCleanupStack()
	ctx := context.Background()

	stack.Add("step", func(ctx context.Context) error {
		return nil
	})

	// First run should work
	stack.Run(ctx)

	// Second run should have no steps
	order := []string{}
	stack.Add("newstep", func(ctx context.Context) error {
		order = append(order, "newstep")
		return nil
	})

	// After Run clears the stack, adding a new step should work
	errs := stack.Run(ctx)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
	if len(order) != 1 || order[0] != "newstep" {
		t.Errorf("expected only newstep, got %v", order)
	}
}

