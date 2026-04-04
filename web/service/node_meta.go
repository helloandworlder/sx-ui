package service

import (
	"github.com/helloandworlder/sx-ui/v2/database"
	"github.com/helloandworlder/sx-ui/v2/database/model"
)

// NodeMetaService provides CRUD for the node-level key-value metadata store.
type NodeMetaService struct{}

// Get returns the value for the given key, or empty string if not found.
func (s *NodeMetaService) Get(key string) (string, error) {
	db := database.GetDB()
	var meta model.NodeMeta
	err := db.Where("key = ?", key).First(&meta).Error
	if err != nil {
		if database.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return meta.Value, nil
}

// Set upserts a key-value pair.
func (s *NodeMetaService) Set(key, value string) error {
	db := database.GetDB()
	var meta model.NodeMeta
	err := db.Where("key = ?", key).First(&meta).Error
	if err != nil {
		if database.IsNotFound(err) {
			return db.Create(&model.NodeMeta{Key: key, Value: value}).Error
		}
		return err
	}
	meta.Value = value
	return db.Save(&meta).Error
}

// GetAll returns all key-value pairs as a map.
func (s *NodeMetaService) GetAll() (map[string]string, error) {
	db := database.GetDB()
	var metas []model.NodeMeta
	err := db.Find(&metas).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(metas))
	for _, m := range metas {
		result[m.Key] = m.Value
	}
	return result, nil
}

// Delete removes a key.
func (s *NodeMetaService) Delete(key string) error {
	db := database.GetDB()
	return db.Where("key = ?", key).Delete(&model.NodeMeta{}).Error
}
