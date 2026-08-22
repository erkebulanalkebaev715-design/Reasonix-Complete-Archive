package control

import "fmt"

// SetStrictPreCallBudget toggles the executor's pre-network hard-budget guard.
// It is idle-only for the same reason as SetTaskCostBudget: an APK policy change
// must not alter admission rules underneath an in-flight provider request.
func (c *Controller) SetStrictPreCallBudget(enabled bool) error {
	if c == nil {
		return fmt.Errorf("controller is nil")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running || c.finishing {
		return ErrTurnRunning
	}
	applied := false
	if c.executor != nil {
		c.executor.SetStrictPreCallBudget(enabled)
		applied = true
	}
	if r, ok := c.runner.(interface{ SetStrictPreCallBudget(bool) }); ok {
		r.SetStrictPreCallBudget(enabled)
		applied = true
	}
	if enabled && !applied {
		return fmt.Errorf("controller has no strict pre-call budget target")
	}
	return nil
}

// StrictPreCallBudget reports the executor's current host-owned hard-budget
// guard state. It is content-free and exists so frontends/tests can verify that
// a controller rebuild did not silently drop the admission policy.
func (c *Controller) StrictPreCallBudget() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.executor != nil && c.executor.StrictPreCallBudget()
}
