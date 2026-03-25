package pub

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/ethanmoffat/eolib-go/v3/data"
	eopub "github.com/ethanmoffat/eolib-go/v3/protocol/pub"
	eopubsrv "github.com/ethanmoffat/eolib-go/v3/protocol/pub/server"
)

// Global pub file databases, loaded at startup.
var (
	ItemDB  *eopub.Eif
	NpcDB   *eopub.Enf
	SpellDB *eopub.Esf
	ClassDB *eopub.Ecf
	DropDB  *eopubsrv.DropFile

	// npcDropIndex provides O(1) lookup for NPC drop tables.
	npcDropIndex map[int][]eopubsrv.DropRecord

	// mapMetaCache caches map RID and file size to avoid disk I/O per login.
	mapMetaCache   map[int]mapMeta
)

type mapMeta struct {
	rid  []int
	size int
}

// LoadAll loads all pub files from the data/pub directory.
func LoadAll() error {
	var err error

	ItemDB, err = loadEIF("data/pub/dat001.eif")
	if err != nil {
		return fmt.Errorf("loading EIF: %w", err)
	}
	slog.Info("EIF loaded", "items", len(ItemDB.Items))

	NpcDB, err = loadENF("data/pub/dtn001.enf")
	if err != nil {
		return fmt.Errorf("loading ENF: %w", err)
	}
	slog.Info("ENF loaded", "npcs", len(NpcDB.Npcs))

	SpellDB, err = loadESF("data/pub/dsl001.esf")
	if err != nil {
		return fmt.Errorf("loading ESF: %w", err)
	}
	slog.Info("ESF loaded", "spells", len(SpellDB.Skills))

	ClassDB, err = loadECF("data/pub/dat001.ecf")
	if err != nil {
		return fmt.Errorf("loading ECF: %w", err)
	}
	slog.Info("ECF loaded", "classes", len(ClassDB.Classes))

	DropDB, err = loadDropFile("data/data/dat001.edf")
	if err != nil {
		slog.Warn("failed to load drop file (non-fatal)", "err", err)
		DropDB = &eopubsrv.DropFile{}
	} else {
		slog.Info("EDF loaded", "npc_drop_tables", len(DropDB.Npcs))
	}

	// Build O(1) NPC drop index
	npcDropIndex = make(map[int][]eopubsrv.DropRecord, len(DropDB.Npcs))
	for _, npc := range DropDB.Npcs {
		npcDropIndex[npc.NpcId] = npc.Drops
	}

	// Cache map metadata to avoid disk reads on login
	mapMetaCache = make(map[int]mapMeta)
	loadMapMetaCache()

	return nil
}

// GetItem returns the EIF record for the given item ID (1-indexed), or nil.
func GetItem(id int) *eopub.EifRecord {
	if ItemDB == nil || id < 1 || id > len(ItemDB.Items) {
		return nil
	}
	return &ItemDB.Items[id-1]
}

// GetNpc returns the ENF record for the given NPC ID (1-indexed), or nil.
func GetNpc(id int) *eopub.EnfRecord {
	if NpcDB == nil || id < 1 || id > len(NpcDB.Npcs) {
		return nil
	}
	return &NpcDB.Npcs[id-1]
}

// GetSpell returns the ESF record for the given spell ID (1-indexed), or nil.
func GetSpell(id int) *eopub.EsfRecord {
	if SpellDB == nil || id < 1 || id > len(SpellDB.Skills) {
		return nil
	}
	return &SpellDB.Skills[id-1]
}

// GetClass returns the ECF record for the given class ID (1-indexed), or nil.
func GetClass(id int) *eopub.EcfRecord {
	if ClassDB == nil || id < 1 || id > len(ClassDB.Classes) {
		return nil
	}
	return &ClassDB.Classes[id-1]
}

// GetNpcDrops returns the drop records for an NPC using O(1) index lookup.
func GetNpcDrops(npcID int) []eopubsrv.DropRecord {
	return npcDropIndex[npcID]
}

// Pub file RID/length helpers for welcome packets.

func EifRid() []int {
	if ItemDB != nil && len(ItemDB.Rid) >= 2 {
		return ItemDB.Rid[:2]
	}
	return []int{0, 0}
}

func EifLength() int {
	if ItemDB != nil {
		return len(ItemDB.Items)
	}
	return 0
}

func EnfRid() []int {
	if NpcDB != nil && len(NpcDB.Rid) >= 2 {
		return NpcDB.Rid[:2]
	}
	return []int{0, 0}
}

func EnfLength() int {
	if NpcDB != nil {
		return len(NpcDB.Npcs)
	}
	return 0
}

func EsfRid() []int {
	if SpellDB != nil && len(SpellDB.Rid) >= 2 {
		return SpellDB.Rid[:2]
	}
	return []int{0, 0}
}

func EsfLength() int {
	if SpellDB != nil {
		return len(SpellDB.Skills)
	}
	return 0
}

func EcfRid() []int {
	if ClassDB != nil && len(ClassDB.Rid) >= 2 {
		return ClassDB.Rid[:2]
	}
	return []int{0, 0}
}

func EcfLength() int {
	if ClassDB != nil {
		return len(ClassDB.Classes)
	}
	return 0
}

func MapRid(mapID int) []int {
	if m, ok := mapMetaCache[mapID]; ok {
		return m.rid
	}
	return []int{0, 0}
}

func MapFileSize(mapID int) int {
	if m, ok := mapMetaCache[mapID]; ok {
		return m.size
	}
	return 0
}

func loadMapMetaCache() {
	entries, err := os.ReadDir("data/maps")
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) < 5 {
			continue
		}
		ext := name[len(name)-4:]
		if ext != ".emf" && ext != ".EMF" {
			continue
		}
		base := name[:len(name)-4]
		id, err := strconv.Atoi(base)
		if err != nil {
			continue
		}
		path := fmt.Sprintf("data/maps/%s", name)
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var rid []int
		if len(raw) >= 4 {
			rid = []int{int(raw[0]) | int(raw[1])<<8, int(raw[2]) | int(raw[3])<<8}
		} else {
			rid = []int{0, 0}
		}
		mapMetaCache[id] = mapMeta{rid: rid, size: len(raw)}
	}
}

// ItemGraphicID returns the display graphic for a visible equipment item.
// For equipment, this is the Spec1 field (not GraphicId) per the EO protocol.
func ItemGraphicID(itemID int) int {
	if itemID == 0 {
		return 0
	}
	item := GetItem(itemID)
	if item == nil {
		return 0
	}
	return item.Spec1
}

func loadDropFile(path string) (*eopubsrv.DropFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	reader := data.NewEoReader(raw)
	var df eopubsrv.DropFile
	if err := df.Deserialize(reader); err != nil {
		return nil, err
	}
	return &df, nil
}

func loadEIF(path string) (*eopub.Eif, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	reader := data.NewEoReader(raw)
	var eif eopub.Eif
	if err := eif.Deserialize(reader); err != nil {
		return nil, err
	}
	return &eif, nil
}

func loadENF(path string) (*eopub.Enf, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	reader := data.NewEoReader(raw)
	var enf eopub.Enf
	if err := enf.Deserialize(reader); err != nil {
		return nil, err
	}
	return &enf, nil
}

func loadESF(path string) (*eopub.Esf, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	reader := data.NewEoReader(raw)
	var esf eopub.Esf
	if err := esf.Deserialize(reader); err != nil {
		return nil, err
	}
	return &esf, nil
}

func loadECF(path string) (*eopub.Ecf, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	reader := data.NewEoReader(raw)
	var ecf eopub.Ecf
	if err := ecf.Deserialize(reader); err != nil {
		return nil, err
	}
	return &ecf, nil
}
