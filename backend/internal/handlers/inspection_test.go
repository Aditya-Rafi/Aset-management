package handlers_test

import (
	"bytes"
	"strings"
	"testing"
)

func laptopTx(e *env, typ string, emp int, fromEmp int, assetID int, insp map[string]interface{}) (int, map[string]interface{}) {
	body := map[string]interface{}{"type": typ, "employee_id": emp, "items": []map[string]interface{}{{"asset_id": assetID, "inspection": insp}}}
	if fromEmp != 0 {
		body["from_employee_id"] = fromEmp
	}
	return e.do("POST", "/api/transactions", body)
}

func TestInspectionRules(t *testing.T) {
	e := newEnv(t)
	budi := e.emp("Budi")
	lap := e.asset("LPT", nil)
	mon := e.asset("MON", nil)
	mouse := e.asset("MSE", nil)
	post := func(items ...map[string]interface{}) (int, map[string]interface{}) {
		return e.do("POST", "/api/transactions", map[string]interface{}{"type": "out", "employee_id": budi, "items": items})
	}

	// the checklist is served so the form never drifts from the server rules
	_, m := e.do("GET", "/api/inspection-templates", nil)
	lt := data(m)["laptop"].(map[string]interface{})
	if len(lt["sections"].([]interface{})) != 2 || len(lt["accessories"].([]interface{})) != 7 {
		t.Fatalf("laptop template: %v", lt)
	}

	// laptop and monitor have a checklist, a mouse does not
	if code, m := post(map[string]interface{}{"asset_id": num(lap["id"])}); code != 422 || !strings.Contains(str(m["message"]), "LPT-0001: pemeriksaan fisik belum diisi") {
		t.Fatalf("missing inspection: %d %v", code, m)
	}
	if code, _ := post(map[string]interface{}{"asset_id": num(mouse["id"])}); code != 201 {
		t.Fatalf("a mouse needs no inspection: %d", code)
	}
	bad := map[string]map[string]interface{}{
		"skip without reason": {"skipped": true},
		"nothing answered":    {"items": []map[string]interface{}{}, "conclusion": "layak"},
		"no conclusion":       {"items": []map[string]interface{}{{"code": "hw_body", "result": "ok"}}},
		"unknown item":        {"items": []map[string]interface{}{{"code": "hw_nope", "result": "ok"}}, "conclusion": "layak"},
		"bad without note":    {"items": []map[string]interface{}{{"code": "hw_body", "result": "bad"}}, "conclusion": "perlu_perbaikan"},
		"bad result":          {"items": []map[string]interface{}{{"code": "hw_body", "result": "maybe"}}, "conclusion": "layak"},
		"battery 150%":        {"items": []map[string]interface{}{{"code": "hw_body", "result": "ok"}}, "conclusion": "layak", "battery_health": 150},
	}
	for name, insp := range bad {
		if code, m := post(map[string]interface{}{"asset_id": num(lap["id"]), "inspection": insp}); code != 422 {
			t.Fatalf("%s should be rejected, got %d %v", name, code, m)
		}
	}
	// nothing was saved by the failed attempts
	_, m = e.do("GET", "/api/assets/"+itoa(num(lap["id"])), nil)
	if data(m)["status"] != "available" {
		t.Fatalf("failed documents must not change the asset")
	}

	// a general-profile asset takes the short checklist
	general := map[string]interface{}{"conclusion": "layak", "items": []map[string]interface{}{
		{"code": "gen_body", "result": "ok"}, {"code": "gen_function", "result": "ok"}, {"code": "gen_parts", "result": "bad", "note": "kabel putus"},
	}}
	if code, m := post(map[string]interface{}{"asset_id": num(mon["id"]), "inspection": general}); code != 201 {
		t.Fatalf("monitor with short checklist: %d %v", code, m)
	}
	// a skipped check is allowed with a reason, and is recorded as skipped
	code, m := post(map[string]interface{}{"asset_id": num(lap["id"]), "inspection": map[string]interface{}{"skipped": true, "skip_reason": "Unit dipinjam darurat"}})
	if code != 201 {
		t.Fatalf("skip with reason: %d %v", code, m)
	}
	txID := itoa(num(data(m)["id"]))
	_, m = e.do("GET", "/api/transactions/"+txID, nil)
	ins := data(m)["inspections"].([]interface{})
	if len(ins) != 1 || ins[0].(map[string]interface{})["skipped"] != true || ins[0].(map[string]interface{})["skip_reason"] != "Unit dipinjam darurat" {
		t.Fatalf("skipped inspection: %v", ins)
	}
	// the BAST of that document says so
	res, b := e.get("/api/transactions/" + txID + "/bast")
	if res.StatusCode != 200 || !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Fatalf("bast: %d", res.StatusCode)
	}
	// the category decides who needs a checklist
	cid := itoa(e.cat("MSE"))
	_, m = e.do("GET", "/api/categories/"+cid, nil)
	cat := data(m)
	cat["inspection"] = "general"
	if code, m := e.do("PUT", "/api/categories/"+cid, cat); code != 200 || data(m)["inspection"] != "general" {
		t.Fatalf("category inspection: %d %v", code, m)
	}
	cat["inspection"] = "weird"
	if code, _ := e.do("PUT", "/api/categories/"+cid, cat); code != 422 {
		t.Fatalf("unknown profile must be rejected: %d", code)
	}
}

// Rizal gets a laptop, returns it with a cracked screen -> the damage is his. Then it
// goes to Sari already cracked and she returns it with a broken hinge too.
func TestResponsibilityReport(t *testing.T) {
	e := newEnv(t)
	rizal, sari := e.emp("Rizal"), e.emp("Sari")
	lap := e.asset("LPT", map[string]interface{}{"brand": "Lenovo"})
	id := num(lap["id"])
	aid := itoa(id)

	// 1) handed to Rizal, everything fine, battery 95
	i1 := inspLaptop()
	i1["battery_health"] = 95
	if code, m := laptopTx(e, "out", rizal, 0, id, i1); code != 201 {
		t.Fatalf("out: %d %v", code, m)
	}
	// 2) Rizal returns it: screen is bad (new), battery 80; the check says repair
	i2 := inspLaptop("hw_screen")
	i2["battery_health"] = 80
	code, m := laptopTx(e, "in", rizal, 0, id, i2)
	if code != 201 {
		t.Fatalf("in: %d %v", code, m)
	}
	// the conclusion decides the state when none is given: perlu_perbaikan -> Diservis
	_, m = e.do("GET", "/api/assets/"+aid, nil)
	if data(m)["status"] != "repair" {
		t.Fatalf("conclusion should send the asset to repair, got %v", data(m)["status"])
	}
	// back to available after service, then handed to Sari with the screen still bad
	e.do("PUT", "/api/assets/"+aid+"/status", map[string]interface{}{"status": "available", "note": "diservis"})
	if code, m := laptopTx(e, "out", sari, 0, id, inspLaptop("hw_screen")); code != 201 {
		t.Fatalf("out to sari: %d %v", code, m)
	}
	// 3) Sari returns it: the hinge is newly bad
	if code, m := laptopTx(e, "in", sari, 0, id, inspLaptop("hw_screen", "hw_hinge")); code != 201 {
		t.Fatalf("in from sari: %d %v", code, m)
	}

	_, m = e.do("GET", "/api/assets/"+aid+"/condition-report", nil)
	rep := data(m)
	if len(rep["events"].([]interface{})) != 4 || rep["disclaimer"] == "" {
		t.Fatalf("report events: %v", rep)
	}
	type f struct{ typ, who, kind string }
	got := map[string]f{}
	for _, x := range rep["findings"].([]interface{}) {
		fm := x.(map[string]interface{})
		r := fm["responsible"].(map[string]interface{})
		got[str(fm["type"])+"/"+str(fm["code"])] = f{str(fm["type"]), str(r["name"]), str(r["kind"])}
	}
	// screen broke while Rizal had it
	if g := got["new_damage/hw_screen"]; g.who != "Rizal" || g.kind != "employee" {
		t.Fatalf("screen damage must be Rizal's: %+v (%v)", g, got)
	}
	// the hinge broke while Sari had it
	if g := got["new_damage/hw_hinge"]; g.who != "Sari" {
		t.Fatalf("hinge damage must be Sari's: %+v", g)
	}
	// Sari received it with the screen already bad: not hers
	if g, ok := got["existing/hw_screen"]; !ok || g.kind == "employee" && g.who == "Sari" {
		t.Fatalf("pre-existing screen damage must not be blamed on Sari: %+v %v", g, ok)
	}
	// battery dropped 15 points under Rizal
	if g := got["battery/hw_battery"]; g.who != "Rizal" {
		t.Fatalf("battery loss: %+v", g)
	}
	// service period: the screen was Tidak Baik at Rizal's return and Sari got it still bad
	// -> reported as existing, and nothing says it was fixed
	if _, ok := got["fixed/hw_screen"]; ok {
		t.Fatalf("screen was never fixed")
	}
}

func TestTransferBetweenEmployees(t *testing.T) {
	e := newEnv(t)
	mumu, rizal, sari := e.emp("Mumu"), e.emp("Rizal"), e.emp("Sari")
	lap := e.asset("LPT", nil)
	id := num(lap["id"])
	aid := itoa(id)

	// only assets in use can be transferred, and from the person who holds them
	if code, _ := laptopTx(e, "transfer", rizal, mumu, id, inspLaptop()); code != 422 {
		t.Fatalf("transfer of an available asset: %d", code)
	}
	if code, m := laptopTx(e, "out", mumu, 0, id, inspLaptop()); code != 201 {
		t.Fatalf("out: %d %v", code, m)
	}
	if code, m := laptopTx(e, "transfer", rizal, sari, id, inspLaptop()); code != 422 || !strings.Contains(str(m["message"]), "tidak dipegang") {
		t.Fatalf("wrong previous holder: %d %v", code, m)
	}
	if code, _ := laptopTx(e, "transfer", mumu, mumu, id, inspLaptop()); code != 422 {
		t.Fatalf("same person: %d", code)
	}
	if code, _ := laptopTx(e, "transfer", rizal, 0, id, inspLaptop()); code != 422 {
		t.Fatalf("missing from_employee_id: %d", code)
	}

	// Mumu hands it to Rizal; the screen is already cracked at that moment
	code, m := laptopTx(e, "transfer", rizal, mumu, id, inspLaptop("hw_screen"))
	if code != 201 || !strings.HasPrefix(str(data(m)["order_number"]), "TRF-") {
		t.Fatalf("transfer: %d %v", code, m)
	}
	trf := data(m)
	trfID := itoa(num(trf["id"]))
	if trf["from_employee"].(map[string]interface{})["name"] != "Mumu" {
		t.Fatalf("from_employee: %v", trf["from_employee"])
	}
	_, m = e.do("GET", "/api/assets/"+aid, nil)
	if data(m)["status"] != "in_use" || num(data(m)["holder_id"]) != rizal {
		t.Fatalf("holder after transfer: %v", data(m))
	}
	// the screen was good when Mumu got it and bad when he passed it on -> Mumu's period
	_, m = e.do("GET", "/api/assets/"+aid+"/condition-report", nil)
	var found bool
	for _, x := range data(m)["findings"].([]interface{}) {
		fm := x.(map[string]interface{})
		if fm["type"] == "new_damage" && fm["code"] == "hw_screen" && fm["responsible"].(map[string]interface{})["name"] == "Mumu" {
			found = true
		}
	}
	if !found {
		t.Fatalf("damage found at the transfer belongs to the previous holder: %v", data(m)["findings"])
	}

	// BAST and the simple form both render; parties are the two employees
	for _, path := range []string{"/bast", "/pdf"} {
		res, b := e.get("/api/transactions/" + trfID + path)
		if res.StatusCode != 200 || !bytes.HasPrefix(b, []byte("%PDF")) {
			t.Fatalf("%s: %d", path, res.StatusCode)
		}
	}
	// the previous holder cannot be deleted while holding nothing? (Mumu holds nothing now: fine)
	// undo restores Mumu as the holder
	if code, _ := e.do("DELETE", "/api/transactions/"+trfID, nil); code != 200 {
		t.Fatalf("undo transfer: %d", code)
	}
	_, m = e.do("GET", "/api/assets/"+aid, nil)
	if num(data(m)["holder_id"]) != mumu || data(m)["status"] != "in_use" {
		t.Fatalf("undo must hand it back to Mumu: %v", data(m))
	}
	// the undone document leaves no inspection behind
	_, m = e.do("GET", "/api/assets/"+aid+"/condition-report", nil)
	if n := len(data(m)["events"].([]interface{})); n != 1 {
		t.Fatalf("only the original out-check should remain, got %d", n)
	}
}
