package admin

import (
	"log/slog"
	"net/http"
	"strconv"

	pubdata "github.com/avdoseferovic/geoserv/internal/pub"
	eopubsrv "github.com/ethanmoffat/eolib-go/v3/protocol/pub/server"
)

// UI Handlers for Editing Dialogs and POSTs

func (s *Server) executeTemplate(w http.ResponseWriter, name string, data any) bool {
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		slog.Error("template execute error", "name", name, "err", err)
		return false
	}

	return true
}

func parseFormOrBadRequest(w http.ResponseWriter, r *http.Request) bool {
	if err := r.ParseForm(); err != nil {
		slog.Error("parse form error", "err", err)
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return false
	}

	return true
}

// --- Talk ---

func (s *Server) handleUITalkEdit(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	var npc talkNpc
	if pubdata.TalkDB != nil {
		for _, n := range pubdata.TalkDB.Npcs {
			if n.NpcId == id {
				npc = talkNpc{NpcID: n.NpcId, NpcName: npcName(n.NpcId), Rate: n.Rate, Messages: make([]string, len(n.Messages))}
				for i, m := range n.Messages {
					npc.Messages[i] = m.Message
				}
				break
			}
		}
	}
	if npc.NpcID == 0 {
		npc.NpcID = id
		npc.NpcName = npcName(id)
	}
	s.executeTemplate(w, "talk_edit", npc)
}

func (s *Server) handleUITalkPost(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	if !parseFormOrBadRequest(w, r) {
		return
	}
	if id == 0 {
		id, _ = strconv.Atoi(r.FormValue("new_id"))
	}
	rate, _ := strconv.Atoi(r.FormValue("rate"))
	msgs := r.PostForm["messages"]

	if pubdata.TalkDB == nil {
		pubdata.TalkDB = &eopubsrv.TalkFile{}
	}
	found := false
	for i, n := range pubdata.TalkDB.Npcs {
		if n.NpcId == id {
			pubdata.TalkDB.Npcs[i].Rate = rate
			pubdata.TalkDB.Npcs[i].Messages = make([]eopubsrv.TalkMessageRecord, len(msgs))
			for j, m := range msgs {
				pubdata.TalkDB.Npcs[i].Messages[j] = eopubsrv.TalkMessageRecord{Message: m}
			}
			found = true
			break
		}
	}
	if !found {
		rec := eopubsrv.TalkRecord{NpcId: id, Rate: rate, Messages: make([]eopubsrv.TalkMessageRecord, len(msgs))}
		for j, m := range msgs {
			rec.Messages[j] = eopubsrv.TalkMessageRecord{Message: m}
		}
		pubdata.TalkDB.Npcs = append(pubdata.TalkDB.Npcs, rec)
	}
	if err := pubdata.SaveTalk(pubdata.TalkDB); err != nil {
		slog.Error("Failed to save talk", "err", err)
	}

	npc := talkNpc{NpcID: id, NpcName: npcName(id), Rate: rate, Messages: msgs}
	s.executeTemplate(w, "talk_row", npc)
}

// --- Drops ---

func (s *Server) handleUIDropsEdit(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	var npc dropNpc
	if pubdata.DropDB != nil {
		for _, n := range pubdata.DropDB.Npcs {
			if n.NpcId == id {
				drops := make([]dropItem, len(n.Drops))
				for j, d := range n.Drops {
					drops[j] = dropItem{ItemID: d.ItemId, ItemName: itemName(d.ItemId), MinAmount: d.MinAmount, MaxAmount: d.MaxAmount, Rate: d.Rate}
				}
				npc = dropNpc{NpcID: n.NpcId, NpcName: npcName(n.NpcId), Drops: drops}
				break
			}
		}
	}
	if npc.NpcID == 0 {
		npc.NpcID = id
		npc.NpcName = npcName(id)
	}
	s.executeTemplate(w, "drop_edit", npc)
}

func (s *Server) handleUIDropsPost(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	if !parseFormOrBadRequest(w, r) {
		return
	}
	if id == 0 {
		id, _ = strconv.Atoi(r.FormValue("new_id"))
	}

	itemIDs := r.PostForm["item_id"]
	minAmounts := r.PostForm["min_amount"]
	maxAmounts := r.PostForm["max_amount"]
	rates := r.PostForm["rate"]

	drops := make([]dropItem, 0, len(itemIDs))
	pubDrops := make([]eopubsrv.DropRecord, 0, len(itemIDs))

	for i := range itemIDs {
		itemID, _ := strconv.Atoi(itemIDs[i])
		minAmt, _ := strconv.Atoi(minAmounts[i])
		maxAmt, _ := strconv.Atoi(maxAmounts[i])
		rate, _ := strconv.Atoi(rates[i])

		if itemID == 0 {
			continue
		}

		drops = append(drops, dropItem{ItemID: itemID, ItemName: itemName(itemID), MinAmount: minAmt, MaxAmount: maxAmt, Rate: rate})
		pubDrops = append(pubDrops, eopubsrv.DropRecord{ItemId: itemID, MinAmount: minAmt, MaxAmount: maxAmt, Rate: rate})
	}

	if pubdata.DropDB == nil {
		pubdata.DropDB = &eopubsrv.DropFile{}
	}

	found := false
	for i, n := range pubdata.DropDB.Npcs {
		if n.NpcId == id {
			pubdata.DropDB.Npcs[i].Drops = pubDrops
			found = true
			break
		}
	}
	if !found {
		pubdata.DropDB.Npcs = append(pubdata.DropDB.Npcs, eopubsrv.DropNpcRecord{NpcId: id, Drops: pubDrops})
	}

	if err := pubdata.SaveDrops(pubdata.DropDB); err != nil {
		slog.Error("Failed to save drops", "err", err)
	}

	npc := dropNpc{NpcID: id, NpcName: npcName(id), Drops: drops}
	s.executeTemplate(w, "drop_row", npc)
}

// --- Inns ---

func (s *Server) handleUIInnsEdit(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	var inn innRecord
	if pubdata.InnDB != nil {
		for _, i := range pubdata.InnDB.Inns {
			if i.BehaviorId == id {
				qs := make([]innQuestion, len(i.Questions))
				for j, q := range i.Questions {
					qs[j] = innQuestion{Question: q.Question, Answer: q.Answer}
				}
				inn = innRecord{
					BehaviorID: i.BehaviorId, VendorName: vendorName(i.BehaviorId), Name: i.Name,
					SpawnMap: i.SpawnMap, SpawnX: i.SpawnX, SpawnY: i.SpawnY,
					SleepMap: i.SleepMap, SleepX: i.SleepX, SleepY: i.SleepY,
					AltSpawnEnabled: i.AlternateSpawnEnabled,
					AltSpawnMap:     i.AlternateSpawnMap, AltSpawnX: i.AlternateSpawnX, AltSpawnY: i.AlternateSpawnY,
					Questions: qs,
				}
				break
			}
		}
	}
	if inn.BehaviorID == 0 {
		inn.BehaviorID = id
		inn.VendorName = vendorName(id)
	}
	s.executeTemplate(w, "inn_edit", inn)
}

func (s *Server) handleUIInnsPost(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	if !parseFormOrBadRequest(w, r) {
		return
	}
	if id == 0 {
		id, _ = strconv.Atoi(r.FormValue("new_id"))
	}

	name := r.FormValue("name")
	spawnMap, _ := strconv.Atoi(r.FormValue("spawn_map"))
	spawnX, _ := strconv.Atoi(r.FormValue("spawn_x"))
	spawnY, _ := strconv.Atoi(r.FormValue("spawn_y"))
	sleepMap, _ := strconv.Atoi(r.FormValue("sleep_map"))
	sleepX, _ := strconv.Atoi(r.FormValue("sleep_x"))
	sleepY, _ := strconv.Atoi(r.FormValue("sleep_y"))
	altSpawnEnabled := r.FormValue("alt_spawn_enabled") == "true"
	altSpawnMap, _ := strconv.Atoi(r.FormValue("alt_spawn_map"))
	altSpawnX, _ := strconv.Atoi(r.FormValue("alt_spawn_x"))
	altSpawnY, _ := strconv.Atoi(r.FormValue("alt_spawn_y"))

	questions := r.PostForm["question"]
	answers := r.PostForm["answer"]

	qs := make([]innQuestion, 0, len(questions))
	pubQs := make([]eopubsrv.InnQuestionRecord, 0, len(questions))
	for i := range questions {
		qs = append(qs, innQuestion{Question: questions[i], Answer: answers[i]})
		pubQs = append(pubQs, eopubsrv.InnQuestionRecord{Question: questions[i], Answer: answers[i]})
	}

	if pubdata.InnDB == nil {
		pubdata.InnDB = &eopubsrv.InnFile{}
	}

	found := false
	for i, inn := range pubdata.InnDB.Inns {
		if inn.BehaviorId == id {
			pubdata.InnDB.Inns[i].Name = name
			pubdata.InnDB.Inns[i].SpawnMap = spawnMap
			pubdata.InnDB.Inns[i].SpawnX = spawnX
			pubdata.InnDB.Inns[i].SpawnY = spawnY
			pubdata.InnDB.Inns[i].SleepMap = sleepMap
			pubdata.InnDB.Inns[i].SleepX = sleepX
			pubdata.InnDB.Inns[i].SleepY = sleepY
			pubdata.InnDB.Inns[i].AlternateSpawnEnabled = altSpawnEnabled
			pubdata.InnDB.Inns[i].AlternateSpawnMap = altSpawnMap
			pubdata.InnDB.Inns[i].AlternateSpawnX = altSpawnX
			pubdata.InnDB.Inns[i].AlternateSpawnY = altSpawnY
			pubdata.InnDB.Inns[i].Questions = pubQs
			found = true
			break
		}
	}

	if !found {
		pubdata.InnDB.Inns = append(pubdata.InnDB.Inns, eopubsrv.InnRecord{
			BehaviorId: id, Name: name,
			SpawnMap: spawnMap, SpawnX: spawnX, SpawnY: spawnY,
			SleepMap: sleepMap, SleepX: sleepX, SleepY: sleepY,
			AlternateSpawnEnabled: altSpawnEnabled,
			AlternateSpawnMap:     altSpawnMap, AlternateSpawnX: altSpawnX, AlternateSpawnY: altSpawnY,
			Questions: pubQs,
		})
	}

	if err := pubdata.SaveInns(pubdata.InnDB); err != nil {
		slog.Error("Failed to save inns", "err", err)
	}

	inn := innRecord{
		BehaviorID: id, VendorName: vendorName(id), Name: name,
		SpawnMap: spawnMap, SpawnX: spawnX, SpawnY: spawnY,
		SleepMap: sleepMap, SleepX: sleepX, SleepY: sleepY,
		AltSpawnEnabled: altSpawnEnabled,
		AltSpawnMap:     altSpawnMap, AltSpawnX: altSpawnX, AltSpawnY: altSpawnY,
		Questions: qs,
	}
	s.executeTemplate(w, "inn_row", inn)
}

// --- Shops ---

func (s *Server) handleUIShopsEdit(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	var shop shopRecord
	if pubdata.ShopFileDB != nil {
		for _, sh := range pubdata.ShopFileDB.Shops {
			if sh.BehaviorId == id {
				var trades []shopTrade
				for _, t := range sh.Trades {
					if t.ItemId != 0 {
						trades = append(trades, shopTrade{ItemID: t.ItemId, ItemName: itemName(t.ItemId), BuyPrice: t.BuyPrice, SellPrice: t.SellPrice, MaxAmount: t.MaxAmount})
					}
				}
				var crafts []shopCraft
				for _, c := range sh.Crafts {
					if c.ItemId != 0 {
						var ings []shopIngredient
						for _, ing := range c.Ingredients {
							if ing.ItemId != 0 {
								ings = append(ings, shopIngredient{ItemID: ing.ItemId, ItemName: itemName(ing.ItemId), Amount: ing.Amount})
							}
						}
						crafts = append(crafts, shopCraft{ItemID: c.ItemId, ItemName: itemName(c.ItemId), Ingredients: ings})
					}
				}
				shop = shopRecord{BehaviorID: sh.BehaviorId, VendorName: vendorName(sh.BehaviorId), Name: sh.Name, Trades: trades, Crafts: crafts}
				break
			}
		}
	}
	if shop.BehaviorID == 0 {
		shop.BehaviorID = id
		shop.VendorName = vendorName(id)
	}
	s.executeTemplate(w, "shop_edit", shop)
}

func (s *Server) handleUIShopsPost(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	if !parseFormOrBadRequest(w, r) {
		return
	}
	if id == 0 {
		id, _ = strconv.Atoi(r.FormValue("new_id"))
	}

	name := r.FormValue("name")

	tradeItemIDs := r.PostForm["trade_item_id"]
	tradeBuys := r.PostForm["trade_buy"]
	tradeSells := r.PostForm["trade_sell"]
	tradeMaxs := r.PostForm["trade_max"]

	var trades []shopTrade
	var pubTrades []eopubsrv.ShopTradeRecord
	for i := range tradeItemIDs {
		itemID, _ := strconv.Atoi(tradeItemIDs[i])
		if itemID == 0 {
			continue
		}
		buy, _ := strconv.Atoi(tradeBuys[i])
		sell, _ := strconv.Atoi(tradeSells[i])
		maxAmt, _ := strconv.Atoi(tradeMaxs[i])
		trades = append(trades, shopTrade{ItemID: itemID, ItemName: itemName(itemID), BuyPrice: buy, SellPrice: sell, MaxAmount: maxAmt})
		pubTrades = append(pubTrades, eopubsrv.ShopTradeRecord{ItemId: itemID, BuyPrice: buy, SellPrice: sell, MaxAmount: maxAmt})
	}

	craftItemIDs := r.PostForm["craft_item_id"]
	ingIDs := r.PostForm["ing_id"]
	ingAmts := r.PostForm["ing_amt"]

	var crafts []shopCraft
	var pubCrafts []eopubsrv.ShopCraftRecord

	// Each craft always has exactly 4 ingredients in the form due to padding.
	for i := range craftItemIDs {
		itemID, _ := strconv.Atoi(craftItemIDs[i])
		if itemID == 0 {
			continue
		}
		var ings []shopIngredient
		var pubIngs []eopubsrv.ShopCraftIngredientRecord
		for j := range 4 {
			idx := i*4 + j
			if idx < len(ingIDs) {
				ingID, _ := strconv.Atoi(ingIDs[idx])
				ingAmt, _ := strconv.Atoi(ingAmts[idx])
				if ingID != 0 {
					ings = append(ings, shopIngredient{ItemID: ingID, ItemName: itemName(ingID), Amount: ingAmt})
				}
				// eopubsrv always needs array of 4, even if zero
				pubIngs = append(pubIngs, eopubsrv.ShopCraftIngredientRecord{ItemId: ingID, Amount: ingAmt})
			}
		}
		crafts = append(crafts, shopCraft{ItemID: itemID, ItemName: itemName(itemID), Ingredients: ings})
		pubCrafts = append(pubCrafts, eopubsrv.ShopCraftRecord{ItemId: itemID, Ingredients: pubIngs})
	}

	if pubdata.ShopFileDB == nil {
		pubdata.ShopFileDB = &eopubsrv.ShopFile{}
	}

	found := false
	for i, sh := range pubdata.ShopFileDB.Shops {
		if sh.BehaviorId == id {
			pubdata.ShopFileDB.Shops[i].Name = name
			pubdata.ShopFileDB.Shops[i].Trades = pubTrades
			pubdata.ShopFileDB.Shops[i].Crafts = pubCrafts
			found = true
			break
		}
	}
	if !found {
		pubdata.ShopFileDB.Shops = append(pubdata.ShopFileDB.Shops, eopubsrv.ShopRecord{
			BehaviorId: id, Name: name, Trades: pubTrades, Crafts: pubCrafts,
		})
	}

	if err := pubdata.SaveShops(pubdata.ShopFileDB); err != nil {
		slog.Error("Failed to save shops", "err", err)
	}

	if r.PathValue("id") == "0" {
		s.handleUIShops(w, r)
		return
	}
	shop := shopRecord{BehaviorID: id, VendorName: vendorName(id), Name: name, Trades: trades, Crafts: crafts}
	s.executeTemplate(w, "shop_row", shop)
}

// --- Masters ---

func (s *Server) handleUIMastersEdit(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	var master masterRecord
	if pubdata.SkillMasterDB != nil {
		for _, m := range pubdata.SkillMasterDB.SkillMasters {
			if m.BehaviorId == id {
				var skills []skillEntry
				for _, sk := range m.Skills {
					if sk.SkillId != 0 {
						skills = append(skills, skillEntry{
							SkillID: sk.SkillId, LevelReq: sk.LevelRequirement, ClassReq: sk.ClassRequirement,
							Price: sk.Price, SkillReqs: sk.SkillRequirements,
							StrReq: sk.StrRequirement, IntReq: sk.IntRequirement, WisReq: sk.WisRequirement,
							AgiReq: sk.AgiRequirement, ConReq: sk.ConRequirement, ChaReq: sk.ChaRequirement,
						})
					}
				}
				master = masterRecord{BehaviorID: m.BehaviorId, VendorName: vendorName(m.BehaviorId), Name: m.Name, Skills: skills}
				break
			}
		}
	}
	if master.BehaviorID == 0 {
		master.BehaviorID = id
		master.VendorName = vendorName(id)
	}
	s.executeTemplate(w, "master_edit", master)
}

func (s *Server) handleUIMastersPost(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	if !parseFormOrBadRequest(w, r) {
		return
	}
	if id == 0 {
		id, _ = strconv.Atoi(r.FormValue("new_id"))
	}

	name := r.FormValue("name")

	skillIDs := r.PostForm["skill_id"]
	prices := r.PostForm["price"]
	levelReqs := r.PostForm["level_req"]
	classReqs := r.PostForm["class_req"]
	strReqs := r.PostForm["str_req"]
	intReqs := r.PostForm["int_req"]
	wisReqs := r.PostForm["wis_req"]
	agiReqs := r.PostForm["agi_req"]
	conReqs := r.PostForm["con_req"]
	chaReqs := r.PostForm["cha_req"]

	skillReqsFlat := r.PostForm["skill_req"]

	var skills []skillEntry
	var pubSkills []eopubsrv.SkillMasterSkillRecord

	for i := range skillIDs {
		skillID, _ := strconv.Atoi(skillIDs[i])
		if skillID == 0 {
			continue
		}
		price, _ := strconv.Atoi(prices[i])
		levelReq, _ := strconv.Atoi(levelReqs[i])
		classReq, _ := strconv.Atoi(classReqs[i])
		strReq, _ := strconv.Atoi(strReqs[i])
		intReq, _ := strconv.Atoi(intReqs[i])
		wisReq, _ := strconv.Atoi(wisReqs[i])
		agiReq, _ := strconv.Atoi(agiReqs[i])
		conReq, _ := strconv.Atoi(conReqs[i])
		chaReq, _ := strconv.Atoi(chaReqs[i])

		var reqs []int
		for j := range 4 {
			idx := i*4 + j
			if idx < len(skillReqsFlat) {
				reqID, _ := strconv.Atoi(skillReqsFlat[idx])
				reqs = append(reqs, reqID)
			} else {
				reqs = append(reqs, 0)
			}
		}

		skills = append(skills, skillEntry{
			SkillID: skillID, Price: price, LevelReq: levelReq, ClassReq: classReq,
			StrReq: strReq, IntReq: intReq, WisReq: wisReq, AgiReq: agiReq, ConReq: conReq, ChaReq: chaReq,
			SkillReqs: reqs,
		})

		pubSkills = append(pubSkills, eopubsrv.SkillMasterSkillRecord{
			SkillId: skillID, Price: price, LevelRequirement: levelReq, ClassRequirement: classReq,
			StrRequirement: strReq, IntRequirement: intReq, WisRequirement: wisReq, AgiRequirement: agiReq, ConRequirement: conReq, ChaRequirement: chaReq,
			SkillRequirements: reqs,
		})
	}

	if pubdata.SkillMasterDB == nil {
		pubdata.SkillMasterDB = &eopubsrv.SkillMasterFile{}
	}

	found := false
	for i, m := range pubdata.SkillMasterDB.SkillMasters {
		if m.BehaviorId == id {
			pubdata.SkillMasterDB.SkillMasters[i].Name = name
			pubdata.SkillMasterDB.SkillMasters[i].Skills = pubSkills
			found = true
			break
		}
	}
	if !found {
		pubdata.SkillMasterDB.SkillMasters = append(pubdata.SkillMasterDB.SkillMasters, eopubsrv.SkillMasterRecord{
			BehaviorId: id, Name: name, Skills: pubSkills,
		})
	}

	if err := pubdata.SaveSkillMasters(pubdata.SkillMasterDB); err != nil {
		slog.Error("Failed to save masters", "err", err)
	}

	if r.PathValue("id") == "0" {
		s.handleUIMasters(w, r)
		return
	}
	master := masterRecord{BehaviorID: id, VendorName: vendorName(id), Name: name, Skills: skills}
	s.executeTemplate(w, "master_row", master)
}

func (s *Server) handleUIDropsDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	if pubdata.DropDB != nil {
		for i, n := range pubdata.DropDB.Npcs {
			if n.NpcId == id {
				pubdata.DropDB.Npcs = append(pubdata.DropDB.Npcs[:i], pubdata.DropDB.Npcs[i+1:]...)
				_ = pubdata.SaveDrops(pubdata.DropDB)
				break
			}
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleUITalkDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	if pubdata.TalkDB != nil {
		for i, n := range pubdata.TalkDB.Npcs {
			if n.NpcId == id {
				pubdata.TalkDB.Npcs = append(pubdata.TalkDB.Npcs[:i], pubdata.TalkDB.Npcs[i+1:]...)
				_ = pubdata.SaveTalk(pubdata.TalkDB)
				break
			}
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleUIInnsDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	if pubdata.InnDB != nil {
		for i, inn := range pubdata.InnDB.Inns {
			if inn.BehaviorId == id {
				pubdata.InnDB.Inns = append(pubdata.InnDB.Inns[:i], pubdata.InnDB.Inns[i+1:]...)
				_ = pubdata.SaveInns(pubdata.InnDB)
				break
			}
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleUIShopsDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	if pubdata.ShopFileDB != nil {
		for i, sh := range pubdata.ShopFileDB.Shops {
			if sh.BehaviorId == id {
				pubdata.ShopFileDB.Shops = append(pubdata.ShopFileDB.Shops[:i], pubdata.ShopFileDB.Shops[i+1:]...)
				_ = pubdata.SaveShops(pubdata.ShopFileDB)
				break
			}
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleUIMastersDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	if pubdata.SkillMasterDB != nil {
		for i, m := range pubdata.SkillMasterDB.SkillMasters {
			if m.BehaviorId == id {
				pubdata.SkillMasterDB.SkillMasters = append(pubdata.SkillMasterDB.SkillMasters[:i], pubdata.SkillMasterDB.SkillMasters[i+1:]...)
				_ = pubdata.SaveSkillMasters(pubdata.SkillMasterDB)
				break
			}
		}
	}
	w.WriteHeader(http.StatusOK)
}
