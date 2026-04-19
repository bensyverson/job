package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func runLog(db *sql.DB, shortID string) ([]EventEntry, error) {
	task, err := getTaskByShortID(db, shortID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, fmt.Errorf("task %q not found", shortID)
	}

	return getEventsForTaskTree(db, shortID)
}

func runTail(ctx context.Context, db *sql.DB, shortID string, pollInterval time.Duration, callback func([]EventEntry) error) error {
	task, err := getTaskByShortID(db, shortID)
	if err != nil {
		return err
	}
	if task == nil {
		return fmt.Errorf("task %q not found", shortID)
	}

	var lastID int64
	for {
		events, err := getEventsAfterID(db, shortID, lastID)
		if err != nil {
			return err
		}
		if len(events) > 0 {
			if err := callback(events); err != nil {
				return err
			}
			lastID = events[len(events)-1].ID
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(pollInterval):
		}
	}
}
