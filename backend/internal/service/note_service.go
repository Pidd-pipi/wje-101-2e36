package service

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/wjecoffeetaste/wjecoffeetaste/internal/constants"
	"github.com/wjecoffeetaste/wjecoffeetaste/internal/dto"
	"github.com/wjecoffeetaste/wjecoffeetaste/internal/model"
	"github.com/wjecoffeetaste/wjecoffeetaste/internal/repository"
	"github.com/wjecoffeetaste/wjecoffeetaste/internal/util"
)

// NoteService handles tasting notes.
type NoteService struct {
	repo     *repository.TastingNoteRepository
	beanRepo *repository.CoffeeBeanRepository
	logger   *slog.Logger
}

// NewNoteService creates a NoteService.
func NewNoteService(repo *repository.TastingNoteRepository, beanRepo *repository.CoffeeBeanRepository, logger *slog.Logger) *NoteService {
	return &NoteService{repo: repo, beanRepo: beanRepo, logger: logger}
}

// validateBean verifies the selected bean exists when one is bound.
func (s *NoteService) validateBean(beanID *uint) error {
	if beanID == nil {
		return nil
	}
	if _, err := s.beanRepo.FindByID(*beanID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return util.NewAppError(422, constants.CodeValidationError,
				fmt.Sprintf("TastingNote[coffee_bean_id=%d] create failed: bean not found", *beanID))
		}
		return fmt.Errorf("note bean validate: %w", err)
	}
	return nil
}

// Create adds a note for a user.
func (s *NoteService) Create(userID uint, n *model.TastingNote) (*model.TastingNote, error) {
	if !constants.IsValidRoastLevel(n.RoastLevel) {
		return nil, util.NewAppError(422, constants.CodeValidationError,
			fmt.Sprintf("TastingNote[roast_level=%s] create failed: invalid roast level", n.RoastLevel))
	}
	if err := s.validateBean(n.CoffeeBeanID); err != nil {
		return nil, err
	}
	n.UserID = userID
	if n.FlavorTags == "" {
		n.FlavorTags = "[]"
	}
	if err := s.repo.Create(n); err != nil {
		s.logger.Error(fmt.Sprintf(constants.LogNoteCreateFailed, n.CoffeeName), "error", err)
		return nil, fmt.Errorf("note create: %w", err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogNoteCreateSuccess, n.CoffeeName), "id", n.ID, "coffee_bean_id", n.CoffeeBeanID)
	return n, nil
}

// Get returns a note by id.
func (s *NoteService) Get(id uint) (*model.TastingNote, error) {
	n, err := s.repo.FindByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(404, constants.CodeNotFound, fmt.Sprintf("TastingNote[id=%d] not found", id))
		}
		return nil, fmt.Errorf("note get: %w", err)
	}
	return n, nil
}

// BeanInfo returns the live bean profile bound to a note, or nil when unbound.
func (s *NoteService) BeanInfo(beanID *uint) (*dto.NoteBeanInfo, error) {
	if beanID == nil {
		return nil, nil
	}
	b, err := s.beanRepo.FindByID(*beanID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("note bean info: %w", err)
	}
	return &dto.NoteBeanInfo{ID: b.ID, Name: b.Name, Origin: b.Origin, ProcessMethod: b.ProcessMethod}, nil
}

// BeanInfoMap returns live bean profiles keyed by bean id for a list of notes.
func (s *NoteService) BeanInfoMap(notes []model.TastingNote) (map[uint]dto.NoteBeanInfo, error) {
	ids := make([]uint, 0, len(notes))
	seen := map[uint]bool{}
	for i := range notes {
		if notes[i].CoffeeBeanID != nil && !seen[*notes[i].CoffeeBeanID] {
			seen[*notes[i].CoffeeBeanID] = true
			ids = append(ids, *notes[i].CoffeeBeanID)
		}
	}
	beans, err := s.repo.FindBeanByIDs(ids)
	if err != nil {
		return nil, fmt.Errorf("note bean info map: %w", err)
	}
	out := make(map[uint]dto.NoteBeanInfo, len(beans))
	for _, b := range beans {
		out[b.ID] = dto.NoteBeanInfo{ID: b.ID, Name: b.Name, Origin: b.Origin, ProcessMethod: b.ProcessMethod}
	}
	return out, nil
}

// Update edits a note owned by the user.
func (s *NoteService) Update(userID, id uint, n *model.TastingNote, beanID *uint, beanBindingSet bool) (*model.TastingNote, error) {
	exist, err := s.repo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("note update find: %w", err)
	}
	if exist.UserID != userID {
		return nil, util.NewAppError(403, constants.CodeForbidden,
			fmt.Sprintf("TastingNote[id=%d] update failed: user_id=%d not owner", id, userID))
	}
	if beanBindingSet {
		if err := s.validateBean(beanID); err != nil {
			return nil, err
		}
		exist.CoffeeBeanID = beanID
	}
	if n.CoffeeName != "" {
		exist.CoffeeName = n.CoffeeName
	}
	if n.Origin != "" {
		exist.Origin = n.Origin
	}
	if n.RoastLevel != "" {
		if !constants.IsValidRoastLevel(n.RoastLevel) {
			return nil, util.NewAppError(422, constants.CodeValidationError, "invalid roast level")
		}
		exist.RoastLevel = n.RoastLevel
	}
	if n.FlavorTags != "" {
		exist.FlavorTags = n.FlavorTags
	}
	if n.NotesText != "" {
		exist.NotesText = n.NotesText
	}
	if n.OverallScore > 0 {
		exist.AromaScore = n.AromaScore
		exist.AcidityScore = n.AcidityScore
		exist.BodyScore = n.BodyScore
		exist.OverallScore = n.OverallScore
	}
	if err := s.repo.Update(exist); err != nil {
		return nil, fmt.Errorf("note update: %w", err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogNoteUpdateSuccess, id), "id", id, "coffee_bean_id", exist.CoffeeBeanID)
	return exist, nil
}

// Delete removes a note owned by the user.
func (s *NoteService) Delete(userID, id uint) error {
	n, err := s.repo.FindByID(id)
	if err != nil {
		return fmt.Errorf("note delete find: %w", err)
	}
	if n.UserID != userID {
		return util.NewAppError(403, constants.CodeForbidden,
			fmt.Sprintf("TastingNote[id=%d] delete failed: not owner", id))
	}
	if err := s.repo.Delete(id); err != nil {
		return fmt.Errorf("note delete: %w", err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogNoteDeleteSuccess, id), "id", id)
	return nil
}

// List filters notes.
func (s *NoteService) List(roast, origin, keyword string, hot bool, page, pageSize int) ([]model.TastingNote, int64, error) {
	items, total, err := s.repo.List(roast, origin, keyword, hot, page, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("note list: %w", err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogNoteListSuccess, roast, page), "total", total)
	return items, total, nil
}

// ListByUser returns notes of a user.
func (s *NoteService) ListByUser(userID uint) ([]model.TastingNote, error) {
	items, err := s.repo.ListByUser(userID)
	if err != nil {
		return nil, fmt.Errorf("note list by user: %w", err)
	}
	return items, nil
}

// AvgScore returns the average overall score of a user's notes.
func (s *NoteService) AvgScore(userID uint) (float64, error) {
	avg, err := s.repo.AvgScore(userID)
	if err != nil {
		return 0, fmt.Errorf("note avg score: %w", err)
	}
	return avg, nil
}

// TopOrigins returns the top 3 origins by note count.
func (s *NoteService) TopOrigins(userID uint) ([]string, error) {
	origins, err := s.repo.TopOrigins(userID)
	if err != nil {
		return nil, fmt.Errorf("note top origins: %w", err)
	}
	return origins, nil
}

// BackfillBeanBindings links legacy notes (no bean binding) to beans by
// exact coffee_name match; unmatched notes are left untouched.
func (s *NoteService) BackfillBeanBindings() error {
	notes, err := s.repo.ListUnbound()
	if err != nil {
		return fmt.Errorf("note bean backfill list: %w", err)
	}
	matched, unmatched := 0, 0
	for i := range notes {
		bean, err := s.beanRepo.FindByName(notes[i].CoffeeName)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				unmatched++
				continue
			}
			return fmt.Errorf("note bean backfill match: %w", err)
		}
		if err := s.repo.BindBean(notes[i].ID, bean.ID); err != nil {
			return fmt.Errorf("note bean backfill bind: %w", err)
		}
		matched++
	}
	s.logger.Info(fmt.Sprintf(constants.LogNoteBeanBackfill, matched, unmatched),
		"matched", matched, "unmatched", unmatched)
	return nil
}
