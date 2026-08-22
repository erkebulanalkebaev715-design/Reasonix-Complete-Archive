package control

import "fmt"

// SetTaskCostBudget updates the cost ceiling used by subsequent turns. It is
// intentionally not part of SessionAPI: the Balance mod reaches it through an
// optional capability so other frontends/fakes stay source-compatible.
//
// The update is idle-only. This prevents a UI slider from moving the spend
// boundary under an in-flight provider request.
func (c *Controller) SetTaskCostBudget(cost float64) error {
	if c == nil {
		return fmt.Errorf("controller is nil")
	}
	if cost < 0 {
		cost = 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running || c.finishing {
		return ErrTurnRunning
	}
	c.taskBudget.Cost = cost
	if c.executor != nil {
		c.executor.SetTaskCostBudget(cost)
	}
	return nil
}
