package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server       Server       `toml:"server"`
	Database     Database     `toml:"database"`
	SLN          SLN          `toml:"sln"`
	Account      Account      `toml:"account"`
	SMTP         SMTP         `toml:"smtp"`
	NewCharacter NewCharacter `toml:"new_character"`
	Jail         Jail         `toml:"jail"`
	Rescue       Rescue       `toml:"rescue"`
	World        World        `toml:"world"`
	Bard         Bard         `toml:"bard"`
	Combat       Combat       `toml:"combat"`
	Map          Map          `toml:"map"`
	Character    Character    `toml:"character"`
	NPCs         NPCs         `toml:"npcs"`
	Bank         Bank         `toml:"bank"`
	Limits       Limits       `toml:"limits"`
	Board        Board        `toml:"board"`
	Chest        Chest        `toml:"chest"`
	Jukebox      Jukebox      `toml:"jukebox"`
	Barber       Barber       `toml:"barber"`
	Guild        Guild        `toml:"guild"`
	Marriage     Marriage     `toml:"marriage"`
	Evacuate     Evacuate     `toml:"evacuate"`
	Items        Items        `toml:"items"`
	AutoPickup   AutoPickup   `toml:"auto_pickup"`
}

type Server struct {
	Host                string `toml:"host"`
	Port                string `toml:"port"`
	WebSocketPort       string `toml:"websocket_port"`
	MaxConnections      int    `toml:"max_connections"`
	MaxPlayers          int    `toml:"max_players"`
	MaxConnectionsPerIP int    `toml:"max_connections_per_ip"`
	IPReconnectLimit    int    `toml:"ip_reconnect_limit"`
	HangupDelay         int    `toml:"hangup_delay"`
	MaxLoginAttempts    int    `toml:"max_login_attempts"`
	PingRate            int    `toml:"ping_rate"`
	EnforceSequence     bool   `toml:"enforce_sequence"`
	MinVersion          string `toml:"min_version"`
	MaxVersion          string `toml:"max_version"`
	SaveRate            int    `toml:"save_rate"`
	GeneratePub         bool   `toml:"generate_pub"`
	Lang                string `toml:"lang"`
	AutoAdmin           bool   `toml:"auto_admin"`
}

type Database struct {
	Driver   string `toml:"driver"`
	Host     string `toml:"host"`
	Port     string `toml:"port"`
	Name     string `toml:"name"`
	Username string `toml:"username"`
	Password string `toml:"password"`
}

type SLN struct {
	Enabled    bool   `toml:"enabled"`
	URL        string `toml:"url"`
	Site       string `toml:"site"`
	Hostname   string `toml:"hostname"`
	ServerName string `toml:"server_name"`
	Rate       int    `toml:"rate"`
	Zone       string `toml:"zone"`
}

type Account struct {
	DelayTime         int  `toml:"delay_time"`
	EmailValidation   bool `toml:"email_validation"`
	Recovery          bool `toml:"recovery"`
	RecoveryShowEmail bool `toml:"recovery_show_email"`
	RecoveryMaskEmail bool `toml:"recovery_mask_email"`
	MaxCharacters     int  `toml:"max_characters"`
}

type SMTP struct {
	FromName    string `toml:"from_name"`
	FromAddress string `toml:"from_address"`
	Host        string `toml:"host"`
	Port        int    `toml:"port"`
	Username    string `toml:"username"`
	Password    string `toml:"password"`
}

type NewCharacter struct {
	SpawnMap       int    `toml:"spawn_map"`
	SpawnX         int    `toml:"spawn_x"`
	SpawnY         int    `toml:"spawn_y"`
	SpawnDirection int    `toml:"spawn_direction"`
	Home           string `toml:"home"`
}

type Jail struct {
	Map     int `toml:"map"`
	X       int `toml:"x"`
	Y       int `toml:"y"`
	FreeMap int `toml:"free_map"`
	FreeX   int `toml:"free_x"`
	FreeY   int `toml:"free_y"`
}

type Rescue struct {
	Map int `toml:"map"`
	X   int `toml:"x"`
	Y   int `toml:"y"`
}

type World struct {
	DropDistance      int     `toml:"drop_distance"`
	DropProtectPlayer int     `toml:"drop_protect_player"`
	DropProtectNPC    int     `toml:"drop_protect_npc"`
	RecoverRate       int     `toml:"recover_rate"`
	NPCRecoverRate    int     `toml:"npc_recover_rate"`
	ChestSpawnRate    int     `toml:"chest_spawn_rate"`
	ExpMultiplier     int     `toml:"exp_multiplier"`
	StatPointsPerLvl  int     `toml:"stat_points_per_level"`
	SkillPointsPerLvl int     `toml:"skill_points_per_level"`
	TickRate          int     `toml:"tick_rate"`
	WarpSuckRate      int     `toml:"warp_suck_rate"`
	GhostRate         int     `toml:"ghost_rate"`
	DrainRate         int     `toml:"drain_rate"`
	DrainHPDamage     float64 `toml:"drain_hp_damage"`
	DrainTPDamage     float64 `toml:"drain_tp_damage"`
	QuakeRate         int     `toml:"quake_rate"`
	SpikeRate         int     `toml:"spike_rate"`
	SpikeDamage       float64 `toml:"spike_damage"`
	InfoRevealsDrops  bool    `toml:"info_reveals_drops"`
}

type Bard struct {
	InstrumentItems []int `toml:"instrument_items"`
	MaxNoteID       int   `toml:"max_note_id"`
}

type WeaponRange struct {
	Weapon int  `toml:"weapon"`
	Range  int  `toml:"range"`
	Arrows bool `toml:"arrows"`
}

type Combat struct {
	WeaponRanges  []WeaponRange `toml:"weapon_ranges"`
	EnforceWeight bool          `toml:"enforce_weight"`
}

type Quake struct {
	MinTicks    int `toml:"min_ticks"`
	MaxTicks    int `toml:"max_ticks"`
	MinStrength int `toml:"min_strength"`
	MaxStrength int `toml:"max_strength"`
}

type Map struct {
	Quakes        []Quake `toml:"quakes"`
	DoorCloseRate int     `toml:"door_close_rate"`
	MaxItems      int     `toml:"max_items"`
}

type Character struct {
	MaxNameLength  int `toml:"max_name_length"`
	MaxTitleLength int `toml:"max_title_length"`
	MaxSkin        int `toml:"max_skin"`
	MaxHairStyle   int `toml:"max_hair_style"`
	MaxHairColor   int `toml:"max_hair_color"`
}

type NPCs struct {
	InstantSpawn     bool `toml:"instant_spawn"`
	FreezeOnEmptyMap bool `toml:"freeze_on_empty_map"`
	ChaseDistance    int  `toml:"chase_distance"`
	BoredTimer       int  `toml:"bored_timer"`
	ActRate          int  `toml:"act_rate"`
	Speed0           int  `toml:"speed_0"`
	Speed1           int  `toml:"speed_1"`
	Speed2           int  `toml:"speed_2"`
	Speed3           int  `toml:"speed_3"`
	Speed4           int  `toml:"speed_4"`
	Speed5           int  `toml:"speed_5"`
	Speed6           int  `toml:"speed_6"`
	TalkRate         int  `toml:"talk_rate"`
}

type Bank struct {
	MaxItemAmount   int `toml:"max_item_amount"`
	BaseSize        int `toml:"base_size"`
	SizeStep        int `toml:"size_step"`
	MaxUpgrades     int `toml:"max_upgrades"`
	UpgradeBaseCost int `toml:"upgrade_base_cost"`
	UpgradeCostStep int `toml:"upgrade_cost_step"`
}

type Limits struct {
	MaxBankGold  int `toml:"max_bank_gold"`
	MaxItem      int `toml:"max_item"`
	MaxTrade     int `toml:"max_trade"`
	MaxChest     int `toml:"max_chest"`
	MaxPartySize int `toml:"max_party_size"`
}

type Board struct {
	MaxPosts         int  `toml:"max_posts"`
	MaxUserPosts     int  `toml:"max_user_posts"`
	MaxRecentPosts   int  `toml:"max_recent_posts"`
	RecentPostTime   int  `toml:"recent_post_time"`
	MaxSubjectLength int  `toml:"max_subject_length"`
	MaxPostLength    int  `toml:"max_post_length"`
	DatePosts        bool `toml:"date_posts"`
	AdminBoard       int  `toml:"admin_board"`
	AdminMaxPosts    int  `toml:"admin_max_posts"`
}

type Chest struct {
	Slots int `toml:"slots"`
}

type Jukebox struct {
	Cost       int `toml:"cost"`
	MaxTrackID int `toml:"max_track_id"`
	TrackTimer int `toml:"track_timer"`
}

type Barber struct {
	BaseCost     int `toml:"base_cost"`
	CostPerLevel int `toml:"cost_per_level"`
}

type Guild struct {
	MinPlayers            int    `toml:"min_players"`
	CreateCost            int    `toml:"create_cost"`
	RecruitCost           int    `toml:"recruit_cost"`
	MinTagLength          int    `toml:"min_tag_length"`
	MaxTagLength          int    `toml:"max_tag_length"`
	MaxNameLength         int    `toml:"max_name_length"`
	MaxDescriptionLength  int    `toml:"max_description_length"`
	MaxRankLength         int    `toml:"max_rank_length"`
	DefaultLeaderRankName string `toml:"default_leader_rank_name"`
	DefaultRecruiterRank  string `toml:"default_recruiter_rank_name"`
	DefaultNewMemberRank  string `toml:"default_new_member_rank_name"`
	MinDeposit            int    `toml:"min_deposit"`
	BankMaxGold           int    `toml:"bank_max_gold"`
}

type Marriage struct {
	ApprovalCost              int `toml:"approval_cost"`
	DivorceCost               int `toml:"divorce_cost"`
	FemaleArmorID             int `toml:"female_armor_id"`
	MaleArmorID               int `toml:"male_armor_id"`
	MinLevel                  int `toml:"min_level"`
	MfxID                     int `toml:"mfx_id"`
	RingItemID                int `toml:"ring_item_id"`
	CeremonyStartDelaySeconds int `toml:"ceremony_start_delay_seconds"`
	CelebrationEffectID       int `toml:"celebration_effect_id"`
}

type Evacuate struct {
	SfxID        int `toml:"sfx_id"`
	TimerSeconds int `toml:"timer_seconds"`
	TimerStep    int `toml:"timer_step"`
}

type Items struct {
	InfiniteUseItems []int `toml:"infinite_use_items"`
	ProtectedItems   []int `toml:"protected_items"`
}

type AutoPickup struct {
	Enabled bool `toml:"enabled"`
	Rate    int  `toml:"rate"`
}

func Load(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("loading config %s: %w", path, err)
	}

	// Apply local overrides if present
	localPath := path[:len(path)-len(".toml")] + ".local.toml"
	if _, err := os.Stat(localPath); err == nil {
		if _, err := toml.DecodeFile(localPath, &cfg); err != nil {
			return nil, fmt.Errorf("loading local config %s: %w", localPath, err)
		}
	}

	return &cfg, nil
}
