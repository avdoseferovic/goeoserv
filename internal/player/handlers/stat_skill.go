package handlers

import (
	"log/slog"

	"github.com/avdo/goeoserv/internal/player"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_StatSkill, eonet.PacketAction_Open, handleStatSkillOpen)
	player.Register(eonet.PacketFamily_StatSkill, eonet.PacketAction_Add, handleStatSkillAdd)
	player.Register(eonet.PacketFamily_StatSkill, eonet.PacketAction_Take, handleStatSkillTake)
	player.Register(eonet.PacketFamily_StatSkill, eonet.PacketAction_Remove, handleStatSkillRemove)
	player.Register(eonet.PacketFamily_StatSkill, eonet.PacketAction_Junk, handleStatSkillJunk)
}

// handleStatSkillOpen opens the skill master dialog.
func handleStatSkillOpen(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.StatSkillOpenClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize stat skill open", "id", p.ID, "err", err)
		return nil
	}

	sessionID := p.GenerateSessionID()

	// TODO: Load actual skill master data from SkillMasterFile based on NPC ID
	return p.Bus.SendPacket(&server.StatSkillOpenServerPacket{
		SessionId: sessionID,
		ShopName:  "Skill Master",
	})
}

// handleStatSkillAdd handles stat point allocation or skill point spending.
func handleStatSkillAdd(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.StatSkillAddClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize stat skill add", "id", p.ID, "err", err)
		return nil
	}

	switch pkt.ActionType {
	case client.Train_Stat:
		data, ok := pkt.ActionTypeData.(*client.StatSkillAddActionTypeDataStat)
		if !ok || data == nil {
			return nil
		}

		if p.StatPoints <= 0 {
			return nil
		}

		p.StatPoints--
		switch data.StatId {
		case client.StatId_Str:
			p.Stats.Str++
		case client.StatId_Int:
			p.Stats.Intl++
		case client.StatId_Wis:
			p.Stats.Wis++
		case client.StatId_Agi:
			p.Stats.Agi++
		case client.StatId_Con:
			p.Stats.Con++
		case client.StatId_Cha:
			p.Stats.Cha++
		}

		return p.Bus.SendPacket(&server.StatSkillPlayerServerPacket{
			StatPoints: p.StatPoints,
			Stats: server.CharacterStatsUpdate{
				BaseStats: server.CharacterBaseStats{
					Str:  p.Stats.Str,
					Intl: p.Stats.Intl,
					Wis:  p.Stats.Wis,
					Agi:  p.Stats.Agi,
					Con:  p.Stats.Con,
					Cha:  p.Stats.Cha,
				},
				MaxHp:     p.Stats.MaxHP,
				MaxTp:     p.Stats.MaxTP,
				MaxSp:     0,
				MaxWeight: 70,
				SecondaryStats: server.CharacterSecondaryStats{
					MinDamage: p.Stats.Str / 2,
					MaxDamage: p.Stats.Str,
					Accuracy:  p.Stats.Agi / 2,
					Evade:     p.Stats.Agi / 2,
					Armor:     p.Stats.Con / 2,
				},
			},
		})

	case client.Train_Skill:
		data, ok := pkt.ActionTypeData.(*client.StatSkillAddActionTypeDataSkill)
		if !ok || data == nil {
			return nil
		}

		if p.SkillPoints <= 0 {
			return nil
		}

		// Level up the spell
		for i := range p.Spells {
			if p.Spells[i].ID == data.SpellId {
				p.Spells[i].Level++
				p.SkillPoints--
				return p.Bus.SendPacket(&server.StatSkillAcceptServerPacket{
					SkillPoints: p.SkillPoints,
					Spell:       eonet.Spell{Id: data.SpellId, Level: p.Spells[i].Level},
				})
			}
		}
	}

	return nil
}

// handleStatSkillTake learns a new spell from the skill master.
func handleStatSkillTake(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.StatSkillTakeClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize stat skill take", "id", p.ID, "err", err)
		return nil
	}

	// TODO: Validate session, check requirements, check gold cost

	// Check not already learned
	for _, s := range p.Spells {
		if s.ID == pkt.SpellId {
			return nil
		}
	}

	p.Spells = append(p.Spells, player.SpellState{ID: pkt.SpellId, Level: 1})

	return p.Bus.SendPacket(&server.StatSkillTakeServerPacket{
		SpellId:    pkt.SpellId,
		GoldAmount: p.Inventory[1],
	})
}

// handleStatSkillRemove forgets a spell.
func handleStatSkillRemove(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.StatSkillRemoveClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize stat skill remove", "id", p.ID, "err", err)
		return nil
	}

	for i, s := range p.Spells {
		if s.ID == pkt.SpellId {
			p.Spells = append(p.Spells[:i], p.Spells[i+1:]...)
			return p.Bus.SendPacket(&server.StatSkillRemoveServerPacket{
				SpellId: pkt.SpellId,
			})
		}
	}

	return nil
}

// handleStatSkillJunk resets stats (full stat/skill reset).
func handleStatSkillJunk(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.StatSkillJunkClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize stat skill junk", "id", p.ID, "err", err)
		return nil
	}
	_ = pkt

	// TODO: Reset stats and refund points
	return nil
}
