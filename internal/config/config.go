package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server       Server       `yaml:"server"`
	Database     Database     `yaml:"database"`
	SLN          SLN          `yaml:"sln"`
	Account      Account      `yaml:"account"`
	SMTP         SMTP         `yaml:"smtp"`
	NewCharacter NewCharacter `yaml:"new_character"`
	Jail         Jail         `yaml:"jail"`
	Rescue       Rescue       `yaml:"rescue"`
	World        World        `yaml:"world"`
	Bard         Bard         `yaml:"bard"`
	Combat       Combat       `yaml:"combat"`
	Map          Map          `yaml:"map"`
	Character    Character    `yaml:"character"`
	NPCs         NPCs         `yaml:"npcs"`
	Bank         Bank         `yaml:"bank"`
	Limits       Limits       `yaml:"limits"`
	Board        Board        `yaml:"board"`
	Chest        Chest        `yaml:"chest"`
	Jukebox      Jukebox      `yaml:"jukebox"`
	Barber       Barber       `yaml:"barber"`
	Guild        Guild        `yaml:"guild"`
	Marriage     Marriage     `yaml:"marriage"`
	Evacuate     Evacuate     `yaml:"evacuate"`
	Items        Items        `yaml:"items"`
	AutoPickup   AutoPickup   `yaml:"auto_pickup"`
	Content      Content      `yaml:"content"`
	Arenas       Arenas       `yaml:"arenas"`
}

// Load reads config from a directory containing server.yaml and gameplay.yaml.
// Each file can have a .local.yaml override (e.g. server.local.yaml).
func Load(dir string) (*Config, error) {
	var cfg Config

	files := []string{"server.yaml", "gameplay.yaml"}
	for _, name := range files {
		if err := loadYAML(filepath.Join(dir, name), &cfg); err != nil {
			return nil, err
		}

		// Apply local overrides (e.g. server.local.yaml)
		localName := name[:len(name)-len(".yaml")] + ".local.yaml"
		localPath := filepath.Join(dir, localName)
		if _, err := os.Stat(localPath); err == nil {
			if err := loadYAML(localPath, &cfg); err != nil {
				return nil, err
			}
		}
	}

	return &cfg, nil
}

func loadYAML(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading config %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parsing config %s: %w", path, err)
	}
	return nil
}
