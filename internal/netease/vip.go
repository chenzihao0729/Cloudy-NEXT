package netease

import (
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// A Membership is a NetEase membership the listener holds: 黑胶SVIP,
// 黑胶VIP or 音乐包, its level where NetEase gives one, and when it ends.
type Membership struct {
	Name    string
	Level   int // 0 when NetEase gives none
	Expires time.Time
}

// SVIP reports whether m is 黑胶SVIP, the one that plays 超清母带.
func (m Membership) SVIP() bool { return m.Name == "黑胶SVIP" }

// vipPackage is one of the memberships NetEase's vip info lists.
type vipPackage struct {
	VipCode    int   `json:"vipCode"`
	ExpireTime int64 `json:"expireTime"`
	VipLevel   int   `json:"vipLevel"`
}

type vipInfo struct {
	Code int `json:"code"`
	Data struct {
		Now          int64       `json:"now"`
		RedVipLevel  int         `json:"redVipLevel"`
		Redplus      *vipPackage `json:"redplus"`
		Associator   *vipPackage `json:"associator"`
		MusicPackage *vipPackage `json:"musicPackage"`
	} `json:"data"`
}

// Memberships returns the memberships userID holds now, the highest first:
// 黑胶SVIP, which takes in 黑胶VIP and so stands for it, else 黑胶VIP; 音乐包
// only held alone, as the vinyl memberships take it in too (NetEase lists it
// beside them, ending with them). None held, it returns none.
func Memberships(userID int64) ([]Membership, error) {
	var res vipInfo
	id := strconv.FormatInt(userID, 10)
	err := eapiPost("/api/music-vip-membership/client/vip/info", map[string]any{"userId": id}, &res)
	if err != nil || (res.Code != 200 && res.Code != 0) {
		// The web's own answer, as the desktop's may be refused.
		res = vipInfo{}
		if err = get("/api/music-vip-membership/front/vip/info", url.Values{"userId": {id}}, &res); err != nil {
			return nil, err
		}
	}
	if res.Code != 200 && res.Code != 0 {
		return nil, fmt.Errorf("vip info: code %d", res.Code)
	}
	return memberships(res), nil
}

func memberships(res vipInfo) []Membership {
	d := res.Data
	now := time.Now()
	if d.Now > 0 {
		now = time.UnixMilli(d.Now)
	}
	held := func(p *vipPackage) bool { return p != nil && p.ExpireTime > now.UnixMilli() }
	level := func(p *vipPackage) int {
		if p.VipLevel > 0 {
			return p.VipLevel
		}
		return d.RedVipLevel
	}
	var out []Membership
	switch {
	case held(d.Redplus):
		out = append(out, Membership{Name: "黑胶SVIP", Level: level(d.Redplus), Expires: time.UnixMilli(d.Redplus.ExpireTime)})
	case held(d.Associator):
		out = append(out, Membership{Name: "黑胶VIP", Level: level(d.Associator), Expires: time.UnixMilli(d.Associator.ExpireTime)})
	}
	if len(out) == 0 && held(d.MusicPackage) {
		out = append(out, Membership{Name: "音乐包", Level: d.MusicPackage.VipLevel, Expires: time.UnixMilli(d.MusicPackage.ExpireTime)})
	}
	return out
}
