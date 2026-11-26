package netease

import (
	"encoding/json"
	"testing"
)

// SVIP stands for the 黑胶VIP it takes in; lapsed memberships don't show.
func TestMemberships(t *testing.T) {
	// 音乐包 held with a vinyl membership is part of it, and doesn't show.
	var res vipInfo
	if err := json.Unmarshal([]byte(`{"code":200,"data":{"now":1000,"redVipLevel":7,
		"redplus":{"vipCode":300,"expireTime":5000,"vipLevel":3},
		"associator":{"vipCode":100,"expireTime":5000,"vipLevel":0},
		"musicPackage":{"vipCode":220,"expireTime":500,"vipLevel":2}}}`), &res); err != nil {
		t.Fatal(err)
	}
	ms := memberships(res)
	if len(ms) != 1 || ms[0].Name != "黑胶SVIP" || ms[0].Level != 3 || !ms[0].SVIP() {
		t.Fatalf("got %+v", ms)
	}
	res.Data.Redplus = nil
	ms = memberships(res)
	if len(ms) != 1 || ms[0].Name != "黑胶VIP" || ms[0].Level != 7 {
		t.Fatalf("without SVIP got %+v", ms)
	}
	res.Data.Associator.ExpireTime = 10
	if ms = memberships(res); len(ms) != 0 {
		t.Fatalf("lapsed got %+v", ms)
	}
	res.Data.MusicPackage.ExpireTime = 5000
	if ms = memberships(res); len(ms) != 1 || ms[0].Name != "音乐包" {
		t.Fatalf("lapsed got %+v", ms)
	}
}
