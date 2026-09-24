package repository

import (
	"gorm.io/gorm"

	"github.com/wjecoffeetaste/wjecoffeetaste/internal/model"
)

// CoffeeBeanRepository handles bean persistence.
type CoffeeBeanRepository struct{ db *gorm.DB }

// NewCoffeeBeanRepository creates the repository.
func NewCoffeeBeanRepository(db *gorm.DB) *CoffeeBeanRepository { return &CoffeeBeanRepository{db: db} }

// Create inserts a bean.
func (r *CoffeeBeanRepository) Create(b *model.CoffeeBean) error { return translate(r.db.Create(b).Error) }

// FindByID locates a bean by id.
func (r *CoffeeBeanRepository) FindByID(id uint) (*model.CoffeeBean, error) {
	var b model.CoffeeBean
	if err := translate(r.db.First(&b, id).Error); err != nil {
		return nil, err
	}
	return &b, nil
}

// Update persists a bean.
func (r *CoffeeBeanRepository) Update(b *model.CoffeeBean) error { return translate(r.db.Save(b).Error) }

// Delete removes a bean.
func (r *CoffeeBeanRepository) Delete(id uint) error {
	res := r.db.Delete(&model.CoffeeBean{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// List filters beans by origin/process/keyword.
func (r *CoffeeBeanRepository) List(origin, process, keyword string, page, pageSize int) ([]model.CoffeeBean, int64, error) {
	var items []model.CoffeeBean
	var total int64
	q := r.db.Model(&model.CoffeeBean{})
	if origin != "" {
		q = q.Where("origin = ?", origin)
	}
	if process != "" {
		q = q.Where("process_method = ?", process)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("name LIKE ? OR flavor_tags LIKE ?", like, like)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// BeanWithNoteCount pairs a bean with the number of notes bound to it.
type BeanWithNoteCount struct {
	model.CoffeeBean
	NoteCount int64 `gorm:"column:note_count" json:"note_count"`
}

// ListWithNoteCount filters beans like List and attaches the related note count.
func (r *CoffeeBeanRepository) ListWithNoteCount(origin, process, keyword string, page, pageSize int) ([]BeanWithNoteCount, int64, error) {
	var items []BeanWithNoteCount
	var total int64
	countQ := r.db.Model(&model.CoffeeBean{})
	listQ := r.db.Table("coffee_beans").
		Select("coffee_beans.*, COALESCE(note_counts.cnt, 0) AS note_count").
		Joins("LEFT JOIN (SELECT coffee_bean_id, COUNT(*) AS cnt FROM tasting_notes WHERE coffee_bean_id > 0 GROUP BY coffee_bean_id) AS note_counts ON note_counts.coffee_bean_id = coffee_beans.id")
	if origin != "" {
		countQ = countQ.Where("origin = ?", origin)
		listQ = listQ.Where("coffee_beans.origin = ?", origin)
	}
	if process != "" {
		countQ = countQ.Where("process_method = ?", process)
		listQ = listQ.Where("coffee_beans.process_method = ?", process)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		countQ = countQ.Where("name LIKE ? OR flavor_tags LIKE ?", like, like)
		listQ = listQ.Where("coffee_beans.name LIKE ? OR coffee_beans.flavor_tags LIKE ?", like, like)
	}
	if err := countQ.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := listQ.Order("coffee_beans.id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Scan(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
