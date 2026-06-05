package jobs

// RunOnceExposed exports runOnce for external test packages.
func (c *AutoHealTTLCleanup) RunOnceExposed(ctx context.Context) {
	c.runOnce(ctx)
}
