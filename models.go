package main

import "database/sql"

type Task struct {
	ID             int64
	ShortID        string
	ParentID       *int64
	Title          string
	Description    string
	Status         string
	SortOrder      int
	ClaimedBy      *string
	ClaimExpiresAt *int64
	CompletionNote *string
	CreatedAt      int64
	UpdatedAt      int64
}

type Event struct {
	ID        int64
	TaskID    int64
	EventType string
	Detail    string
	CreatedAt int64
}

type TaskNode struct {
	Task     *Task
	Children []*TaskNode
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTask(s scanner) (*Task, error) {
	var t Task
	var parentID sql.NullInt64
	var claimedBy sql.NullString
	var claimExpiresAt sql.NullInt64
	var completionNote sql.NullString

	err := s.Scan(
		&t.ID, &t.ShortID, &parentID, &t.Title, &t.Description,
		&t.Status, &t.SortOrder, &claimedBy, &claimExpiresAt,
		&completionNote, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if parentID.Valid {
		pid := parentID.Int64
		t.ParentID = &pid
	}
	if claimedBy.Valid {
		cb := claimedBy.String
		t.ClaimedBy = &cb
	}
	if claimExpiresAt.Valid {
		ce := claimExpiresAt.Int64
		t.ClaimExpiresAt = &ce
	}
	if completionNote.Valid {
		cn := completionNote.String
		t.CompletionNote = &cn
	}

	return &t, nil
}
