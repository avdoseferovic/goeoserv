package handlers

import (
	"context"
	"log/slog"

	"github.com/avdo/goeoserv/internal/content"
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

func handleStatSkillOpen(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.StatSkillOpenClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize stat skill open", "id", p.ID, "err", err)
		return nil
	}

	sessionID := p.GenerateSessionID()
	npcID := 0
	if p.World != nil {
		npcID = p.World.GetNpcEnfID(p.MapID, pkt.NpcIndex)
	}
	master, ok := content.GetSkillMaster(npcID)
	if !ok {
		return p.Bus.SendPacket(&server.StatSkillOpenServerPacket{SessionId: sessionID, ShopName: "Skill Master"})
	}

	skills := make([]server.SkillLearn, 0, len(master.Spells))
	for _, spell := range master.Spells {
		reqs := []int{0, 0, 0, 0}
		if spell.RequiredID > 0 {
			reqs[0] = spell.RequiredID
		}
		skills = append(skills, server.SkillLearn{
			Id:                spell.SpellID,
			LevelRequirement:  spell.MinLevel,
			ClassRequirement:  spell.ClassID,
			Cost:              spell.Cost,
			SkillRequirements: reqs,
			StatRequirements:  server.CharacterBaseStats{},
		})
	}

	return p.Bus.SendPacket(&server.StatSkillOpenServerPacket{SessionId: sessionID, ShopName: master.Name, Skills: skills})
}

func handleStatSkillAdd(ctx context.Context, p *player.Player, reader *player.EoReader) error {
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
		if !ok || data == nil || p.StatPoints <= 0 {
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
		p.CalculateStats()
		return p.Bus.SendPacket(&server.StatSkillPlayerServerPacket{StatPoints: p.StatPoints, Stats: statUpdate(p)})

	case client.Train_Skill:
		data, ok := pkt.ActionTypeData.(*client.StatSkillAddActionTypeDataSkill)
		if !ok || data == nil || p.SkillPoints <= 0 {
			return nil
		}
		for i := range p.Spells {
			if p.Spells[i].ID == data.SpellId {
				p.Spells[i].Level++
				p.SkillPoints--
				return p.Bus.SendPacket(&server.StatSkillAcceptServerPacket{SkillPoints: p.SkillPoints, Spell: eonet.Spell{Id: data.SpellId, Level: p.Spells[i].Level}})
			}
		}
	}

	return nil
}

func handleStatSkillTake(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.StatSkillTakeClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize stat skill take", "id", p.ID, "err", err)
		return nil
	}
	if !p.ValidateSessionID(pkt.SessionId) {
		return nil
	}

	learn, ok := findSkillLearn(pkt.SpellId)
	if !ok {
		return nil
	}
	if learn.MinLevel > 0 && p.CharLevel < learn.MinLevel {
		return p.Bus.SendPacket(&server.StatSkillReplyServerPacket{ReplyCode: server.SkillMasterReply_WrongClass})
	}
	if learn.ClassID > 0 && p.ClassID != learn.ClassID {
		return p.Bus.SendPacket(&server.StatSkillReplyServerPacket{ReplyCode: server.SkillMasterReply_WrongClass})
	}
	if learn.RequiredID > 0 && p.Inventory[learn.RequiredID] < learn.RequiredN {
		return p.Bus.SendPacket(&server.StatSkillReplyServerPacket{ReplyCode: server.SkillMasterReply_RemoveItems})
	}
	if !p.RemoveItem(1, learn.Cost) {
		return nil
	}
	if learn.RequiredID > 0 && learn.RequiredN > 0 {
		p.RemoveItem(learn.RequiredID, learn.RequiredN)
	}
	for _, s := range p.Spells {
		if s.ID == pkt.SpellId {
			return nil
		}
	}
	p.Spells = append(p.Spells, player.SpellState{ID: pkt.SpellId, Level: 1})
	return p.Bus.SendPacket(&server.StatSkillTakeServerPacket{SpellId: pkt.SpellId, GoldAmount: p.Inventory[1]})
}

func handleStatSkillRemove(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.StatSkillRemoveClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize stat skill remove", "id", p.ID, "err", err)
		return nil
	}
	if !p.ValidateSessionID(pkt.SessionId) {
		return nil
	}

	for i, s := range p.Spells {
		if s.ID == pkt.SpellId {
			p.Spells = append(p.Spells[:i], p.Spells[i+1:]...)
			return p.Bus.SendPacket(&server.StatSkillRemoveServerPacket{SpellId: pkt.SpellId})
		}
	}
	return nil
}

func handleStatSkillJunk(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.StatSkillJunkClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize stat skill junk", "id", p.ID, "err", err)
		return nil
	}
	if !p.ValidateSessionID(pkt.SessionId) {
		return nil
	}

	refunded := p.Stats.Str + p.Stats.Intl + p.Stats.Wis + p.Stats.Agi + p.Stats.Con + p.Stats.Cha
	p.Stats.Str = 0
	p.Stats.Intl = 0
	p.Stats.Wis = 0
	p.Stats.Agi = 0
	p.Stats.Con = 0
	p.Stats.Cha = 0
	p.StatPoints += refunded
	p.CalculateStats()

	return p.Bus.SendPacket(&server.StatSkillPlayerServerPacket{StatPoints: p.StatPoints, Stats: statUpdate(p)})
}

func findSkillLearn(spellID int) (content.SkillSpell, bool) {
	for _, master := range content.Current().SkillMasters {
		for _, spell := range master.Spells {
			if spell.SpellID == spellID {
				return spell, true
			}
		}
	}
	return content.SkillSpell{}, false
}

func statUpdate(p *player.Player) server.CharacterStatsUpdate {
	return server.CharacterStatsUpdate{
		BaseStats: server.CharacterBaseStats{Str: p.Stats.Str, Intl: p.Stats.Intl, Wis: p.Stats.Wis, Agi: p.Stats.Agi, Con: p.Stats.Con, Cha: p.Stats.Cha},
		MaxHp:     p.CharMaxHP,
		MaxTp:     p.CharMaxTP,
		MaxSp:     p.CharMaxSP,
		MaxWeight: p.MaxWeight,
		SecondaryStats: server.CharacterSecondaryStats{
			MinDamage: p.MinDamage,
			MaxDamage: p.MaxDamage,
			Accuracy:  p.Accuracy,
			Evade:     p.Evade,
			Armor:     p.Armor,
		},
	}
}
