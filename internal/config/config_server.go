package config

import "time"

const defaultAccountDelayTimeMinutes = 15

type Server struct {
	Host                string `yaml:"host"`
	Port                string `yaml:"port"`
	MaxConnections      int    `yaml:"max_connections"`
	MaxPlayers          int    `yaml:"max_players"`
	MaxConnectionsPerIP int    `yaml:"max_connections_per_ip"`
	IPReconnectLimit    int    `yaml:"ip_reconnect_limit"`
	HangupDelay         int    `yaml:"hangup_delay"`
	MaxLoginAttempts    int    `yaml:"max_login_attempts"`
	PingRate            int    `yaml:"ping_rate"`
	EnforceSequence     bool   `yaml:"enforce_sequence"`
	MinVersion          string `yaml:"min_version"`
	MaxVersion          string `yaml:"max_version"`
	SaveRate            int    `yaml:"save_rate"`
	AutoAdmin           bool   `yaml:"auto_admin"`
}

type Database struct {
	Driver   string `yaml:"driver"`
	Host     string `yaml:"host"`
	Port     string `yaml:"port"`
	Name     string `yaml:"name"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type SLN struct {
	Enabled    bool   `yaml:"enabled"`
	URL        string `yaml:"url"`
	Site       string `yaml:"site"`
	Hostname   string `yaml:"hostname"`
	ServerName string `yaml:"server_name"`
	Rate       int    `yaml:"rate"`
	Zone       string `yaml:"zone"`
}

type Account struct {
	DelayTime         int  `yaml:"delay_time"`
	EmailValidation   bool `yaml:"email_validation"`
	Recovery          bool `yaml:"recovery"`
	RecoveryShowEmail bool `yaml:"recovery_show_email"`
	RecoveryMaskEmail bool `yaml:"recovery_mask_email"`
	MaxCharacters     int  `yaml:"max_characters"`
}

func (a Account) DelayMinutes() int {
	if a.DelayTime > 0 {
		return a.DelayTime
	}
	return defaultAccountDelayTimeMinutes
}

func (a Account) DelayDuration() time.Duration {
	return time.Duration(a.DelayMinutes()) * time.Minute
}

type SMTP struct {
	FromName    string `yaml:"from_name"`
	FromAddress string `yaml:"from_address"`
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	Username    string `yaml:"username"`
	Password    string `yaml:"password"`
}

type NewCharacter struct {
	SpawnMap       int    `yaml:"spawn_map"`
	SpawnX         int    `yaml:"spawn_x"`
	SpawnY         int    `yaml:"spawn_y"`
	SpawnDirection int    `yaml:"spawn_direction"`
	Home           string `yaml:"home"`
}

type Jail struct {
	Map     int `yaml:"map"`
	X       int `yaml:"x"`
	Y       int `yaml:"y"`
	FreeMap int `yaml:"free_map"`
	FreeX   int `yaml:"free_x"`
	FreeY   int `yaml:"free_y"`
}

type Rescue struct {
	Map int `yaml:"map"`
	X   int `yaml:"x"`
	Y   int `yaml:"y"`
}
