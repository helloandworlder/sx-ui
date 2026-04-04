package service

import (
	"github.com/helloandworlder/sx-ui/v2/database"
	"github.com/helloandworlder/sx-ui/v2/database/model"
)

// RoutingCrudService provides CRUD for the new RoutingRule model.
type RoutingCrudService struct {
	ConfigSeqService ConfigSeqService
}

func (s *RoutingCrudService) GetAll() ([]model.RoutingRule, error) {
	db := database.GetDB()
	var rules []model.RoutingRule
	err := db.Order("priority ASC").Find(&rules).Error
	return rules, err
}

func (s *RoutingCrudService) GetEnabled() ([]model.RoutingRule, error) {
	db := database.GetDB()
	var rules []model.RoutingRule
	err := db.Where("enabled = ?", true).Order("priority ASC").Find(&rules).Error
	return rules, err
}

func (s *RoutingCrudService) GetById(id int) (*model.RoutingRule, error) {
	db := database.GetDB()
	var rule model.RoutingRule
	err := db.First(&rule, id).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func (s *RoutingCrudService) Create(rule *model.RoutingRule) error {
	db := database.GetDB()
	seq, err := s.ConfigSeqService.BumpSeqAndHash()
	if err != nil {
		return err
	}
	rule.Seq = seq
	return db.Create(rule).Error
}

func (s *RoutingCrudService) Update(rule *model.RoutingRule) error {
	db := database.GetDB()
	seq, err := s.ConfigSeqService.BumpSeqAndHash()
	if err != nil {
		return err
	}
	rule.Seq = seq
	return db.Save(rule).Error
}

func (s *RoutingCrudService) Delete(id int) error {
	db := database.GetDB()
	_, err := s.ConfigSeqService.BumpSeqAndHash()
	if err != nil {
		return err
	}
	return db.Delete(&model.RoutingRule{}, id).Error
}

// Reorder accepts a slice of {id, priority} and bulk-updates.
func (s *RoutingCrudService) Reorder(items []struct {
	Id       int `json:"id"`
	Priority int `json:"priority"`
}) error {
	db := database.GetDB()
	tx := db.Begin()
	for _, item := range items {
		if err := tx.Model(&model.RoutingRule{}).Where("id = ?", item.Id).
			Update("priority", item.Priority).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	if err := tx.Commit().Error; err != nil {
		return err
	}
	// Bump seq after commit to avoid SQLite locking conflict
	_, err := s.ConfigSeqService.BumpSeqAndHash()
	return err
}
