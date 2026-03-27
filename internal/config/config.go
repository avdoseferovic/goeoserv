package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

const defaultAccountDelayTimeMinutes = 15

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

type Server struct {
	Host                string `yaml:"host"`
	Port                string `yaml:"port"`
	WebSocketPort       string `yaml:"websocket_port"`
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

type World struct {
	DropDistance      int     `yaml:"drop_distance"`
	DropProtectPlayer int     `yaml:"drop_protect_player"`
	DropProtectNPC    int     `yaml:"drop_protect_npc"`
	RecoverRate       int     `yaml:"recover_rate"`
	NPCRecoverRate    int     `yaml:"npc_recover_rate"`
	ChestSpawnRate    int     `yaml:"chest_spawn_rate"`
	ExpMultiplier     int     `yaml:"exp_multiplier"`
	StatPointsPerLvl  int     `yaml:"stat_points_per_level"`
	SkillPointsPerLvl int     `yaml:"skill_points_per_level"`
	TickRate          int     `yaml:"tick_rate"`
	WarpSuckRate      int     `yaml:"warp_suck_rate"`
	GhostRate         int     `yaml:"ghost_rate"`
	DrainRate         int     `yaml:"drain_rate"`
	DrainHPDamage     float64 `yaml:"drain_hp_damage"`
	DrainTPDamage     float64 `yaml:"drain_tp_damage"`
	QuakeRate         int     `yaml:"quake_rate"`
	SpikeRate         int     `yaml:"spike_rate"`
	SpikeDamage       float64 `yaml:"spike_damage"`
	InfoRevealsDrops  bool    `yaml:"info_reveals_drops"`
}

type Bard struct {
	InstrumentItems []int `yaml:"instrument_items"`
	MaxNoteID       int   `yaml:"max_note_id"`
}

type WeaponRange struct {
	Weapon int  `yaml:"weapon"`
	Range  int  `yaml:"range"`
	Arrows bool `yaml:"arrows"`
}

type Combat struct {
	WeaponRanges  []WeaponRange `yaml:"weapon_ranges"`
	EnforceWeight bool          `yaml:"enforce_weight"`
}

type Quake struct {
	MinTicks    int `yaml:"min_ticks"`
	MaxTicks    int `yaml:"max_ticks"`
	MinStrength int `yaml:"min_strength"`
	MaxStrength int `yaml:"max_strength"`
}

type Map struct {
	Quakes        []Quake `yaml:"quakes"`
	DoorCloseRate int     `yaml:"door_close_rate"`
	MaxItems      int     `yaml:"max_items"`
}

type Character struct {
	MaxNameLength  int `yaml:"max_name_length"`
	MaxTitleLength int `yaml:"max_title_length"`
	MaxSkin        int `yaml:"max_skin"`
	MaxHairStyle   int `yaml:"max_hair_style"`
	MaxHairColor   int `yaml:"max_hair_color"`
}

type NPCs struct {
	InstantSpawn     bool `yaml:"instant_spawn"`
	FreezeOnEmptyMap bool `yaml:"freeze_on_empty_map"`
	ChaseDistance    int  `yaml:"chase_distance"`
	BoredTimer       int  `yaml:"bored_timer"`
	ActRate          int  `yaml:"act_rate"`
	Speed0           int  `yaml:"speed_0"`
	Speed1           int  `yaml:"speed_1"`
	Speed2           int  `yaml:"speed_2"`
	Speed3           int  `yaml:"speed_3"`
	Speed4           int  `yaml:"speed_4"`
	Speed5           int  `yaml:"speed_5"`
	Speed6           int  `yaml:"speed_6"`
	TalkRate         int  `yaml:"talk_rate"`
}

type Bank struct {
	MaxItemAmount   int `yaml:"max_item_amount"`
	BaseSize        int `yaml:"base_size"`
	SizeStep        int `yaml:"size_step"`
	MaxUpgrades     int `yaml:"max_upgrades"`
	UpgradeBaseCost int `yaml:"upgrade_base_cost"`
	UpgradeCostStep int `yaml:"upgrade_cost_step"`
}

type Limits struct {
	MaxBankGold  int `yaml:"max_bank_gold"`
	MaxItem      int `yaml:"max_item"`
	MaxTrade     int `yaml:"max_trade"`
	MaxChest     int `yaml:"max_chest"`
	MaxPartySize int `yaml:"max_party_size"`
}

type Board struct {
	MaxPosts         int  `yaml:"max_posts"`
	MaxUserPosts     int  `yaml:"max_user_posts"`
	MaxRecentPosts   int  `yaml:"max_recent_posts"`
	RecentPostTime   int  `yaml:"recent_post_time"`
	MaxSubjectLength int  `yaml:"max_subject_length"`
	MaxPostLength    int  `yaml:"max_post_length"`
	DatePosts        bool `yaml:"date_posts"`
	AdminBoard       int  `yaml:"admin_board"`
	AdminMaxPosts    int  `yaml:"admin_max_posts"`
}

type Chest struct {
	Slots int `yaml:"slots"`
}

type Jukebox struct {
	Cost       int `yaml:"cost"`
	MaxTrackID int `yaml:"max_track_id"`
	TrackTimer int `yaml:"track_timer"`
}

type Barber struct {
	BaseCost     int `yaml:"base_cost"`
	CostPerLevel int `yaml:"cost_per_level"`
}

type Guild struct {
	MinPlayers            int    `yaml:"min_players"`
	CreateCost            int    `yaml:"create_cost"`
	RecruitCost           int    `yaml:"recruit_cost"`
	MinTagLength          int    `yaml:"min_tag_length"`
	MaxTagLength          int    `yaml:"max_tag_length"`
	MaxNameLength         int    `yaml:"max_name_length"`
	MaxDescriptionLength  int    `yaml:"max_description_length"`
	MaxRankLength         int    `yaml:"max_rank_length"`
	DefaultLeaderRankName string `yaml:"default_leader_rank_name"`
	DefaultRecruiterRank  string `yaml:"default_recruiter_rank_name"`
	DefaultNewMemberRank  string `yaml:"default_new_member_rank_name"`
	MinDeposit            int    `yaml:"min_deposit"`
	BankMaxGold           int    `yaml:"bank_max_gold"`
}

type Marriage struct {
	ApprovalCost              int `yaml:"approval_cost"`
	DivorceCost               int `yaml:"divorce_cost"`
	FemaleArmorID             int `yaml:"female_armor_id"`
	MaleArmorID               int `yaml:"male_armor_id"`
	MinLevel                  int `yaml:"min_level"`
	MfxID                     int `yaml:"mfx_id"`
	RingItemID                int `yaml:"ring_item_id"`
	CeremonyStartDelaySeconds int `yaml:"ceremony_start_delay_seconds"`
	CelebrationEffectID       int `yaml:"celebration_effect_id"`
}

type Evacuate struct {
	SfxID        int `yaml:"sfx_id"`
	TimerSeconds int `yaml:"timer_seconds"`
	TimerStep    int `yaml:"timer_step"`
}

type Items struct {
	InfiniteUseItems []int `yaml:"infinite_use_items"`
	ProtectedItems   []int `yaml:"protected_items"`
}

type AutoPickup struct {
	Enabled bool `yaml:"enabled"`
	Rate    int  `yaml:"rate"`
}

type Content struct {
	ShopFile        string `yaml:"shop_file"`
	SkillMasterFile string `yaml:"skill_master_file"`
}

type ArenaCoords struct {
	X int `yaml:"x"`
	Y int `yaml:"y"`
}

type ArenaSpawn struct {
	From ArenaCoords `yaml:"from"`
	To   ArenaCoords `yaml:"to"`
}

type Arena struct {
	Map              int          `yaml:"map"`
	CountdownSeconds int          `yaml:"countdown_seconds"`
	MinPlayers       int          `yaml:"min_players"`
	MaxPlayers       int          `yaml:"max_players"`
	QueueSpawns      []ArenaSpawn `yaml:"queue_spawns"`
}

func (a Arena) CountdownTicks() int {
	if a.CountdownSeconds <= 0 {
		return 5 * 8
	}

	return a.CountdownSeconds * 8
}

func (a Arena) StartPlayerThreshold() int {
	if a.MinPlayers <= 0 {
		return 2
	}

	return a.MinPlayers
}

func (a Arena) ParticipantLimit() int {
	if a.MaxPlayers <= 0 {
		return 0
	}

	return a.MaxPlayers
}

func (a Arena) UsesQueueSpawns() bool {
	return len(a.QueueSpawns) > 0
}

func (a Arena) QueueSpawnAt(x, y int) *ArenaSpawn {
	for i := range a.QueueSpawns {
		spawn := &a.QueueSpawns[i]
		if spawn.From.X == x && spawn.From.Y == y {
			return spawn
		}
	}

	return nil
}

type Arenas struct {
	Maps []Arena `yaml:"maps"`
}

func (a Arenas) MapConfig(mapID int) *Arena {
	for i := range a.Maps {
		arena := &a.Maps[i]
		if arena.Map == mapID {
			return arena
		}
	}

	return nil
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	// Apply local overrides if present
	localPath := path[:len(path)-len(".yaml")] + ".local.yaml"
	if localData, err := os.ReadFile(localPath); err == nil {
		if err := yaml.Unmarshal(localData, &cfg); err != nil {
			return nil, fmt.Errorf("parsing local config %s: %w", localPath, err)
		}
	}

	return &cfg, nil
}
