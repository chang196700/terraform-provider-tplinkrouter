package client

import (
	"context"
	"errors"
	"time"
)

// Save persists the entire router running configuration, including changes made
// outside Terraform. Call under WithLock after a successful mutation and readback.
// It never runs automatically during login, Query, or provider configuration.
func (c *Client) Save(ctx context.Context) error {
	if c.config.ReadOnly {
		return ErrReadOnly
	}
	// The firmware UI permits twelve minutes for the asynchronous save operation.
	ctx, cancel := context.WithTimeout(ctx, 12*time.Minute)
	defer cancel()
	response, err := c.Mutate(ctx, map[string]any{"method": "do", "system": map[string]any{"save_cfg": nil}})
	if err != nil {
		return err
	}
	if status, _ := response["status"].(string); status != "Saved" && status != "Saving" {
		return errors.New("router did not acknowledge configuration saving")
	}
	for {
		result, err := c.Query(ctx, map[string]any{"system": map[string]any{"name": "save_cfg_status"}})
		if err != nil {
			return err
		}
		if system, ok := result["system"].(map[string]any); ok {
			if saveStatus, ok := system["save_cfg_status"].(map[string]any); ok {
				if status, valid := integer(saveStatus["status"]); valid && status == 1 {
					return nil
				}
			}
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
