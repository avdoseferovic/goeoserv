package content

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/avdo/goeoserv/internal/config"
)

type ShopOffer struct {
	ItemID int `json:"item_id"`
	Cost   int `json:"cost"`
}

type CraftOffer struct {
	ItemID       int            `json:"item_id"`
	Cost         int            `json:"cost"`
	Ingredients  []CraftItem    `json:"ingredients"`
	Requirements CraftCondition `json:"requirements"`
}

type CraftItem struct {
	ItemID int `json:"item_id"`
	Amount int `json:"amount"`
}

type CraftCondition struct {
	MinLevel int `json:"min_level"`
	ClassID  int `json:"class_id"`
}

type Shop struct {
	NPCID int          `json:"npc_id"`
	Name  string       `json:"name"`
	Buy   []ShopOffer  `json:"buy"`
	Sell  []ShopOffer  `json:"sell"`
	Craft []CraftOffer `json:"craft"`
}

type SkillSpell struct {
	SpellID    int `json:"spell_id"`
	Cost       int `json:"cost"`
	MinLevel   int `json:"min_level"`
	ClassID    int `json:"class_id"`
	RequiredID int `json:"required_item_id"`
	RequiredN  int `json:"required_item_amount"`
}

type SkillMaster struct {
	NPCID  int          `json:"npc_id"`
	Name   string       `json:"name"`
	Spells []SkillSpell `json:"spells"`
}

type Registry struct {
	Shops        map[int]Shop
	SkillMasters map[int]SkillMaster
}

var current = &Registry{
	Shops:        map[int]Shop{},
	SkillMasters: map[int]SkillMaster{},
}

func Load(cfg *config.Config) (*Registry, error) {
	reg := &Registry{
		Shops:        map[int]Shop{},
		SkillMasters: map[int]SkillMaster{},
	}

	if cfg == nil {
		return reg, nil
	}

	if cfg.Content.ShopFile != "" {
		shops, err := loadJSONFile[[]Shop](cfg.Content.ShopFile)
		if err != nil {
			return nil, fmt.Errorf("loading shop file: %w", err)
		}
		for _, shop := range shops {
			reg.Shops[shop.NPCID] = shop
		}
	}

	if cfg.Content.SkillMasterFile != "" {
		skillMasters, err := loadJSONFile[[]SkillMaster](cfg.Content.SkillMasterFile)
		if err != nil {
			return nil, fmt.Errorf("loading skill master file: %w", err)
		}
		for _, master := range skillMasters {
			reg.SkillMasters[master.NPCID] = master
		}
	}

	current = reg
	return reg, nil
}

func Current() *Registry {
	return current
}

func GetShop(npcID int) (Shop, bool) {
	shop, ok := current.Shops[npcID]
	return shop, ok
}

func GetSkillMaster(npcID int) (SkillMaster, bool) {
	master, ok := current.SkillMasters[npcID]
	return master, ok
}

func loadJSONFile[T any](path string) (T, error) {
	var result T
	data, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, err
	}
	return result, nil
}
