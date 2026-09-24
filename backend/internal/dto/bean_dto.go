package dto

import "time"

// BeanCreateRequest creates/updates a coffee bean.
type BeanCreateRequest struct {
	Name          string `json:"name" binding:"required,max=128"`
	Origin        string `json:"origin" binding:"omitempty,max=128"`
	ProcessMethod string `json:"process_method" binding:"required"`
	FlavorTags    string `json:"flavor_tags"`
	Description   string `json:"description"`
}

// BeanWithNoteCount is a coffee bean plus the number of bound tasting notes.
type BeanWithNoteCount struct {
	ID            uint      `json:"id"`
	Name          string    `json:"name"`
	Origin        string    `json:"origin"`
	ProcessMethod string    `json:"process_method"`
	FlavorTags    string    `json:"flavor_tags"`
	Description   string    `json:"description"`
	CreatedAt     time.Time `json:"created_at"`
	NoteCount     int64     `json:"note_count"`
}
