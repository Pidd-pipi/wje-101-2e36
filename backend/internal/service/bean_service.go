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

// BeanService handles coffee bean library.
type BeanService struct {
	repo   *repository.CoffeeBeanRepository
	logger *slog.Logger
}

// NewBeanService creates a BeanService.
func NewBeanService(repo *repository.CoffeeBeanRepository, logger *slog.Logger) *BeanService {
	return &BeanService{repo: repo, logger: logger}
}

// Create adds a bean (admin).
func (s *BeanService) Create(b *model.CoffeeBean) (*model.CoffeeBean, error) {
	if !constants.IsValidProcessMethod(b.ProcessMethod) {
		return nil, util.NewAppError(422, constants.CodeValidationError,
			fmt.Sprintf("CoffeeBean[process_method=%s] create failed: invalid process method", b.ProcessMethod))
	}
	if b.FlavorTags == "" {
		b.FlavorTags = "[]"
	}
	if err := s.repo.Create(b); err != nil {
		if errors.Is(err, repository.ErrDuplicate) {
			return nil, util.NewAppError(409, constants.CodeConflict,
				fmt.Sprintf("CoffeeBean[name=%s] create failed: name exists", b.Name))
		}
		s.logger.Error(fmt.Sprintf(constants.LogBeanCreateFailed, b.Name), "error", err)
		return nil, fmt.Errorf("bean create: %w", err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogBeanCreateSuccess, b.Name), "id", b.ID)
	return b, nil
}

// Update edits a bean (admin).
func (s *BeanService) Update(id uint, b *model.CoffeeBean) (*model.CoffeeBean, error) {
	exist, err := s.repo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("bean update find: %w", err)
	}
	if b.Name != "" {
		exist.Name = b.Name
	}
	if b.Origin != "" {
		exist.Origin = b.Origin
	}
	if b.ProcessMethod != "" {
		if !constants.IsValidProcessMethod(b.ProcessMethod) {
			return nil, util.NewAppError(422, constants.CodeValidationError, "invalid process method")
		}
		exist.ProcessMethod = b.ProcessMethod
	}
	if b.FlavorTags != "" {
		exist.FlavorTags = b.FlavorTags
	}
	if b.Description != "" {
		exist.Description = b.Description
	}
	if err := s.repo.Update(exist); err != nil {
		if errors.Is(err, repository.ErrDuplicate) {
			return nil, util.NewAppError(409, constants.CodeConflict,
				fmt.Sprintf("CoffeeBean[id=%d] update failed: name exists", id))
		}
		return nil, fmt.Errorf("bean update: %w", err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogBeanUpdateSuccess, id), "id", id)
	return exist, nil
}

// Delete removes a bean (admin). A bean still referenced by tasting notes
// cannot be removed; the reference count is returned in the error message.
func (s *BeanService) Delete(id uint) error {
	if _, err := s.repo.FindByID(id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return util.NewAppError(404, constants.CodeNotFound, fmt.Sprintf("CoffeeBean[id=%d] not found", id))
		}
		return fmt.Errorf("bean delete find: %w", err)
	}
	count, err := s.repo.CountNotes(id)
	if err != nil {
		return fmt.Errorf("bean delete count notes: %w", err)
	}
	if count > 0 {
		s.logger.Warn(fmt.Sprintf(constants.LogBeanDeleteBlocked, id, count), "id", id, "note_count", count)
		return util.NewAppError(409, constants.CodeConflict,
			fmt.Sprintf("CoffeeBean[id=%d] delete failed: %d tasting note(s) still reference it", id, count))
	}
	if err := s.repo.Delete(id); err != nil {
		return fmt.Errorf("bean delete: %w", err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogBeanDeleteSuccess, id), "id", id)
	return nil
}

// List filters beans and attaches bound note counts.
func (s *BeanService) List(origin, process, keyword string, page, pageSize int) ([]dto.BeanWithNoteCount, int64, error) {
	items, total, err := s.repo.List(origin, process, keyword, page, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("bean list: %w", err)
	}
	ids := make([]uint, 0, len(items))
	for _, b := range items {
		ids = append(ids, b.ID)
	}
	counts, err := s.repo.CountNotesByIDs(ids)
	if err != nil {
		return nil, 0, fmt.Errorf("bean list note counts: %w", err)
	}
	out := make([]dto.BeanWithNoteCount, 0, len(items))
	for _, b := range items {
		out = append(out, dto.BeanWithNoteCount{
			ID:            b.ID,
			Name:          b.Name,
			Origin:        b.Origin,
			ProcessMethod: b.ProcessMethod,
			FlavorTags:    b.FlavorTags,
			Description:   b.Description,
			CreatedAt:     b.CreatedAt,
			NoteCount:     counts[b.ID],
		})
	}
	s.logger.Info(fmt.Sprintf(constants.LogBeanListSuccess, process), "total", total)
	return out, total, nil
}
