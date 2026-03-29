package gamemap

import (
	"math"
	"math/rand/v2"

	pubdata "github.com/avdo/goeoserv/internal/pub"
	eoproto "github.com/ethanmoffat/eolib-go/v3/protocol"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func (m *GameMap) npcAttackLocked(npc *NpcState, target *MapCharacter) (server.NpcUpdateAttack, bool) {
	if npc == nil || target == nil || !npc.Alive || target.HP <= 0 {
		return server.NpcUpdateAttack{}, false
	}
	if !npcIsOrthogonallyAdjacent(npc.X, npc.Y, target.X, target.Y) {
		return server.NpcUpdateAttack{}, false
	}

	npcAccuracy := 0
	if npcRec := pubdata.GetNpc(npc.ID); npcRec != nil {
		npcAccuracy = npcRec.Accuracy
	}

	damage := 0
	if npcCombatHitRoll(npcAccuracy, target.Evade) {
		damage = reduceNpcDamageByArmor(npcDamageAmount(npc.ID), target.Armor)
		if damage > target.HP {
			damage = target.HP
		}

		target.HP -= damage
		if target.HP < 0 {
			target.HP = 0
		}
	}

	killed := server.PlayerKilledState_Alive
	if target.HP == 0 {
		killed = server.PlayerKilledState_Killed
	}

	return server.NpcUpdateAttack{
		NpcIndex:     npc.Index,
		Killed:       killed,
		Direction:    eoproto.Direction(npc.Direction),
		PlayerId:     target.PlayerID,
		Damage:       damage,
		HpPercentage: playerHpPercentage(target.HP, target.MaxHP),
	}, true
}

// DamageNpc applies damage to an NPC. Returns (actualDamage, killed, hpPercentage).
func (m *GameMap) DamageNpc(npcIndex, playerID, damage int) (int, bool, int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if npcIndex < 0 || npcIndex >= len(m.npcs) {
		return 0, false, 0
	}

	npc := m.npcs[npcIndex]
	if !npc.Alive {
		return 0, false, 0
	}
	if damage <= 0 {
		return 0, false, m.npcHpPercentageLocked(npc)
	}

	actualDamage := damage
	if actualDamage > npc.HP {
		actualDamage = npc.HP
	}

	// Track opponent with O(1) map lookup; reset bored timer on hit
	if npc.Opponents == nil {
		npc.Opponents = make(map[int]*NpcOpponent)
	}
	if opp, ok := npc.Opponents[playerID]; ok {
		opp.DamageDealt += actualDamage
		opp.BoredTicks = 0 // reset bored on hit
	} else {
		npc.Opponents[playerID] = &NpcOpponent{DamageDealt: actualDamage}
	}

	npc.HP -= actualDamage
	if npc.HP <= 0 {
		npc.HP = 0
		npc.Alive = false
		npc.SpawnTicks = npc.SpawnTime * 8
		npc.Opponents = nil
		return actualDamage, true, 0
	}

	return actualDamage, false, m.npcHpPercentageLocked(npc)
}

func (m *GameMap) GetNpcHpPercentage(npcIndex int) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if npcIndex < 0 || npcIndex >= len(m.npcs) {
		return 0
	}

	npc := m.npcs[npcIndex]
	if !npc.Alive {
		return 0
	}

	return m.npcHpPercentageLocked(npc)
}

func (m *GameMap) npcHpPercentageLocked(npc *NpcState) int {
	if npc == nil || npc.MaxHP <= 0 {
		return 0
	}

	hpPct := int(float64(npc.HP) / float64(npc.MaxHP) * 100)
	if hpPct < 0 {
		return 0
	}
	if hpPct > 100 {
		return 100
	}
	return hpPct
}

// IsNpcAt checks if an alive NPC is at the given coordinates.
// Returns the NPC index or -1.
func (m *GameMap) IsNpcAt(x, y int) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, npc := range m.npcs {
		if npc.Alive && npc.X == x && npc.Y == y {
			return npc.Index
		}
	}
	return -1
}

// npcSpeedForType returns the act rate threshold for a given NPC spawn type.
func (m *GameMap) npcSpeedForType(spawnType int) int {
	switch spawnType {
	case 0:
		return m.cfg.NPCs.Speed0
	case 1:
		return m.cfg.NPCs.Speed1
	case 2:
		return m.cfg.NPCs.Speed2
	case 3:
		return m.cfg.NPCs.Speed3
	case 4:
		return m.cfg.NPCs.Speed4
	case 5:
		return m.cfg.NPCs.Speed5
	case 6:
		return m.cfg.NPCs.Speed6
	case 7:
		return 0 // frozen/static
	default:
		return m.cfg.NPCs.Speed0
	}
}

func (m *GameMap) broadcastNpcWalk(npc *NpcState) {
	for _, ch := range m.players {
		_ = ch.Bus.SendPacket(newNpcPlayerPacket(ch, []server.NpcUpdatePosition{{
			NpcIndex:  npc.Index,
			Coords:    eoproto.Coords{X: npc.X, Y: npc.Y},
			Direction: eoproto.Direction(npc.Direction),
		}}, nil, nil))
	}
}

func (m *GameMap) broadcastNpcAppear(npc *NpcState) {
	// Send via NPC_PLAYER with position update
	for _, ch := range m.players {
		_ = ch.Bus.SendPacket(newNpcPlayerPacket(ch, []server.NpcUpdatePosition{{
			NpcIndex:  npc.Index,
			Coords:    eoproto.Coords{X: npc.X, Y: npc.Y},
			Direction: eoproto.Direction(npc.Direction),
		}}, nil, nil))
	}
}

func (m *GameMap) broadcastNpcAttack(attack server.NpcUpdateAttack) {
	for _, ch := range m.players {
		_ = ch.Bus.SendPacket(newNpcPlayerPacket(ch, nil, []server.NpcUpdateAttack{attack}, nil))
	}
}

func newNpcPlayerPacket(
	player *MapCharacter,
	positions []server.NpcUpdatePosition,
	attacks []server.NpcUpdateAttack,
	chats []server.NpcUpdateChat,
) *server.NpcPlayerServerPacket {
	pkt := &server.NpcPlayerServerPacket{
		Positions: positions,
		Attacks:   attacks,
		Chats:     chats,
	}

	if !npcAttackTargetsPlayer(attacks, player.PlayerID) {
		return pkt
	}

	hp, tp := player.HP, player.TP
	pkt.Hp = &hp
	pkt.Tp = &tp
	return pkt
}

func npcAttackTargetsPlayer(attacks []server.NpcUpdateAttack, playerID int) bool {
	for _, attack := range attacks {
		if attack.PlayerId == playerID {
			return true
		}
	}

	return false
}

func npcDamageAmount(npcID int) int {
	npcRec := pubdata.GetNpc(npcID)
	if npcRec == nil {
		return 1
	}

	damage := npcRec.MinDamage
	if npcRec.MaxDamage > npcRec.MinDamage {
		damage += rand.IntN(npcRec.MaxDamage - npcRec.MinDamage + 1)
	}
	if damage < 1 {
		return 1
	}
	return damage
}

func npcCombatHitRoll(accuracy, evade int) bool {
	return rand.Float64() <= npcCombatHitRate(accuracy, evade)
}

func npcCombatHitRate(accuracy, evade int) float64 {
	hitRate := 0.5
	if accuracy+evade > 0 {
		if evade > 0 {
			hitRate = float64(accuracy) / float64(evade*2)
		} else {
			hitRate = 0.8
		}
	}
	return max(0.5, min(0.8, hitRate))
}

func reduceNpcDamageByArmor(damage, armor int) int {
	if damage < 1 {
		return 1
	}
	if armor > 0 && damage < armor*2 {
		reduced := float64(damage) * math.Pow(float64(damage)/float64(armor*2), 2)
		damage = int(reduced)
	}
	if damage < 1 {
		return 1
	}
	return damage
}

func playerHpPercentage(currentHP, maxHP int) int {
	if maxHP <= 0 {
		return 0
	}

	hpPct := currentHP * 100 / maxHP
	if hpPct < 0 {
		return 0
	}
	if hpPct > 100 {
		return 100
	}
	return hpPct
}
