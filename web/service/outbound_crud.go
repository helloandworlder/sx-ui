package service

import (
	"github.com/helloandworlder/sx-ui/v2/database"
	"github.com/helloandworlder/sx-ui/v2/database/model"
)

// OutboundCrudService provides CRUD for the new Outbound model.
// These outbounds are promoted from the xrayTemplateConfig JSON to DB rows
// so GoSea can manage them individually via the REST API.
type OutboundCrudService struct {
	ConfigSeqService ConfigSeqService
}

func (s *OutboundCrudService) GetAll() ([]model.Outbound, error) {
	db := database.GetDB()
	var outs []model.Outbound
	err := db.Find(&outs).Error
	return outs, err
}

func (s *OutboundCrudService) GetEnabled() ([]model.Outbound, error) {
	db := database.GetDB()
	var outs []model.Outbound
	err := db.Where("enabled = ?", true).Find(&outs).Error
	return outs, err
}

func (s *OutboundCrudService) GetById(id int) (*model.Outbound, error) {
	db := database.GetDB()
	var out model.Outbound
	err := db.First(&out, id).Error
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *OutboundCrudService) GetByTag(tag string) (*model.Outbound, error) {
	db := database.GetDB()
	var out model.Outbound
	err := db.Where("tag = ?", tag).First(&out).Error
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *OutboundCrudService) Create(out *model.Outbound) error {
	db := database.GetDB()
	seq, err := s.ConfigSeqService.BumpSeqAndHash()
	if err != nil {
		return err
	}
	out.Seq = seq
	return db.Create(out).Error
}

func (s *OutboundCrudService) Update(out *model.Outbound) error {
	db := database.GetDB()
	seq, err := s.ConfigSeqService.BumpSeqAndHash()
	if err != nil {
		return err
	}
	out.Seq = seq
	return db.Save(out).Error
}

func (s *OutboundCrudService) Delete(id int) error {
	db := database.GetDB()
	_, err := s.ConfigSeqService.BumpSeqAndHash()
	if err != nil {
		return err
	}
	return db.Delete(&model.Outbound{}, id).Error
}

func (s *OutboundCrudService) DeleteByTag(tag string) error {
	db := database.GetDB()
	_, err := s.ConfigSeqService.BumpSeqAndHash()
	if err != nil {
		return err
	}
	return db.Where("tag = ?", tag).Delete(&model.Outbound{}).Error
}
