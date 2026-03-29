package config

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
