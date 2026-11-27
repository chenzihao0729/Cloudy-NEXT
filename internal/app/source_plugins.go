package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/sourcecatalog"
	"netease-music-mygo/internal/sourceplugin"
)

type sourceManager struct {
	mu       sync.RWMutex
	script   string
	info     sourceplugin.Info
	enabled  bool
	failure  string
	resolver *sourceplugin.Resolver
	logs     []sourceLogEntry
}

type sourceLogEntry struct {
	at      time.Time
	level   string
	message string
}

func newSourceManager() *sourceManager { return &sourceManager{} }

var searchSource = sourcecatalog.Search

func (a *app) preferSource(track netease.Track) bool {
	if track.Source != nil {
		return a.source != nil && a.source.isEnabled()
	}
	if track.NoCopyright() {
		return a.source != nil && a.source.isEnabled()
	}
	if a.playbackQuality() == "jymaster" {
		return false // Confirmed SVIP mastering is resolved by NetEase first.
	}
	if !a.canUseVIPSource() || !track.RequiresVIP() {
		return false
	}
	return true
}

func (a *app) canUseVIPSource() bool {
	return (a.profile == nil || a.profile.VIPType == 0) && a.source != nil && a.source.isEnabled()
}

func (a *app) canPlayTrack(track netease.Track) bool {
	if track.Source != nil {
		return a.source != nil && a.source.isEnabled()
	}
	if track.Playable() || track.LocalPath != "" || (track.ID > 0 && a.downloaded(track.ID)) {
		return true
	}
	if track.RequiresVIP() && a.profile != nil && a.profile.VIPType > 0 {
		return true
	}
	return a.preferSource(track)
}

func (m *sourceManager) configure(script string, info sourceplugin.Info, enabled bool) {
	m.mu.Lock()
	old := m.resolver
	m.resolver = nil
	m.script, m.info, m.enabled, m.failure = script, info, enabled, ""
	m.mu.Unlock()
	if old != nil {
		go old.Close()
	}
}

func (m *sourceManager) setEnabled(enabled bool) {
	m.mu.Lock()
	m.enabled = enabled
	m.mu.Unlock()
}

func (m *sourceManager) setFailure(message string) {
	m.mu.Lock()
	m.failure = message
	m.mu.Unlock()
}

func (m *sourceManager) hasPlugin() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.script != ""
}

func (m *sourceManager) isEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled && m.script != ""
}

func (m *sourceManager) close() {
	m.mu.Lock()
	old := m.resolver
	m.resolver = nil
	m.script, m.enabled = "", false
	m.mu.Unlock()
	if old != nil {
		go old.Close()
	}
}

func (m *sourceManager) Resolve(ctx context.Context, track netease.Track, level string, excluded []string) (*netease.Stream, error) {
	m.mu.RLock()
	enabled, script := m.enabled, m.script
	m.mu.RUnlock()
	if !enabled || script == "" {
		return nil, netease.ErrUnavailable
	}
	if len(excluded) > 0 {
		m.addLog("warn", "播放地址返回 403，跳过来源："+strings.Join(excluded, ", "))
	}
	artists := track.ArtistNames()
	query := strings.TrimSpace(strings.Join(append([]string{track.Name}, trackArtistNames(track)...), " "))
	quality, streamLevel, streamType := "320k", "exhigh", "mp3"
	if level == "lossless" || level == "hires" || level == "jymaster" {
		quality, streamLevel, streamType = "flac", "lossless", "flac"
	}
	if level == "jymaster" {
		m.addLog("info", "超清母带仅支持网易云官方 SVIP 播放；切换插件后尝试普通 FLAC 无损")
	}
	resolver, err := m.getResolver(script)
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	sources := make(map[string]string, len(m.info.Sources))
	for platform, name := range m.info.Sources {
		sources[platform] = name
	}
	m.mu.RUnlock()
	platforms := []string{"wy", "tx", "kg", "kw", "mg", "qs"}
	refs := searchTrackSources(track)
	if track.Source != nil {
		platforms = slices.DeleteFunc(platforms, func(p string) bool { return p == "wy" })
	}
	var failures []string
	for _, platform := range platforms {
		if _, exists := sources[platform]; !exists {
			continue
		}
		if slices.Contains(excluded, platform) || platform == "tx" && slices.Contains(excluded, "qq") {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var matched sourcecatalog.Song
		var direct *netease.SourceTrack
		for i := range refs {
			if refs[i].Platform == platform {
				direct = &refs[i]
				break
			}
		}
		if direct != nil {
			ref := direct
			matched = sourcecatalog.Song{ID: ref.ID, MID: ref.MID, MediaMID: ref.MediaMID, Hash: ref.Hash, Name: track.Name, Artists: trackArtistNames(track), Album: track.Album.Name, Duration: track.Length(), CoverURL: track.Album.PicURL}
		} else if platform == "wy" {
			matched = sourcecatalog.Song{ID: fmt.Sprint(track.ID), Name: track.Name, Artists: trackArtistNames(track), Album: track.Album.Name, Duration: track.Length()}
			m.addLog("info", "网易云：尝试原曲 ID "+matched.ID)
		} else {
			m.addLog("info", sources[platform]+"：搜索“"+query+"”")
			candidates, searchErr := searchSource(ctx, platform, query, 10)
			if searchErr != nil {
				m.addLog("error", sources[platform]+"搜索失败："+searchErr.Error())
				failures = append(failures, sources[platform]+"搜索失败："+searchErr.Error())
				continue
			}
			var ok bool
			matched, ok = sourcecatalog.Match(track, candidates)
			if !ok && strings.TrimSpace(track.Name) != "" {
				m.addLog("info", sources[platform]+"：组合搜索未匹配，改用歌名搜索")
				candidates, searchErr = searchSource(ctx, platform, track.Name, 10)
				if searchErr == nil {
					matched, ok = sourcecatalog.Match(track, candidates)
				} else {
					m.addLog("warn", sources[platform]+"歌名搜索失败："+searchErr.Error())
				}
			}
			baseTitle := sourcecatalog.SearchTitle(track.Name)
			if !ok && baseTitle != "" && baseTitle != strings.TrimSpace(track.Name) {
				baseQuery := strings.TrimSpace(strings.Join(append([]string{baseTitle}, trackArtistNames(track)...), " "))
				m.addLog("info", sources[platform]+"：版本后缀未匹配，改用基础歌名搜索“"+baseQuery+"”")
				candidates, searchErr = searchSource(ctx, platform, baseQuery, 10)
				if searchErr == nil {
					matched, ok = sourcecatalog.Match(track, candidates)
				} else {
					m.addLog("warn", sources[platform]+"基础歌名搜索失败："+searchErr.Error())
				}
			}
			if !ok {
				m.addLog("warn", sources[platform]+"：未找到足够确定的同版本歌曲")
				failures = append(failures, sources[platform]+"：没有找到足够确定的匹配歌曲")
				continue
			}
			m.addLog("info", fmt.Sprintf("%s：匹配成功，歌曲 ID=%s", sources[platform], matched.ID))
		}
		musicInfo := pluginMusicInfo(track, artists, platform, matched)
		m.addLog("info", sources[platform]+"：调用插件解析")
		streamURL, resolveErr := resolver.Resolve(ctx, platform, musicInfo, quality)
		if resolveErr != nil {
			m.addLog("error", sources[platform]+"：插件解析失败："+resolveErr.Error())
			failures = append(failures, sources[platform]+": "+resolveErr.Error())
			continue
		}
		m.addLog("info", sources[platform]+"：插件已返回播放地址")
		source := platform
		sourceID := matched.ID
		if platform == "kg" && matched.Hash != "" {
			sourceID = matched.Hash
		}
		referers := map[string]string{
			"tx": "https://y.qq.com/",
			"kg": "https://www.kugou.com/",
			"kw": "https://www.kuwo.cn/",
			"mg": "https://music.migu.cn/",
			"qs": "https://music.douyin.com/",
		}
		userAgents := map[string]string{
			"tx": "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:80.0) Gecko/20100101 Firefox/80.0",
			"kg": "Android15-1070-11440-46-0-DiscoveryDRADProtocol-wifi",
		}
		if platform == "tx" {
			source = "qq"
		}
		return &netease.Stream{
			URL: streamURL, Type: streamType, Level: streamLevel,
			Source: source, SourceID: sourceID, CoverURL: matched.CoverURL, Referer: referers[platform], UserAgent: userAgents[platform],
		}, nil
	}
	if len(failures) == 0 {
		return nil, errors.New("插件中没有可用的音乐来源")
	}
	return nil, fmt.Errorf("所有播放源均不可用：%s", strings.Join(failures, "；"))
}

func (m *sourceManager) addLog(level, message string) {
	message = sourceplugin.RedactLog(message)
	m.mu.Lock()
	m.logs = append(m.logs, sourceLogEntry{at: time.Now(), level: level, message: message})
	if len(m.logs) > 300 {
		m.logs = append([]sourceLogEntry(nil), m.logs[len(m.logs)-300:]...)
	}
	m.mu.Unlock()
}

func (m *sourceManager) logEntries() []sourceLogEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]sourceLogEntry(nil), m.logs...)
}

func (m *sourceManager) clearLogs() {
	m.mu.Lock()
	m.logs = nil
	m.mu.Unlock()
}

func pluginMusicInfo(track netease.Track, artists, platform string, matched sourcecatalog.Song) map[string]any {
	info := map[string]any{
		"id": matched.ID, "songId": matched.ID, "name": track.Name, "songName": track.Name,
		"artist": artists, "singer": artists, "artists": matched.Artists,
		"album": matched.Album, "albumName": matched.Album,
		"interval": int(matched.Duration.Seconds()), "source": platform,
		"rid": matched.ID,
	}
	if matched.Hash != "" {
		info["hash"] = matched.Hash
	}
	if matched.MID != "" {
		info["songmid"], info["mid"], info["musicId"] = matched.MID, matched.MID, matched.MID
	}
	if matched.MediaMID != "" {
		info["media_mid"], info["mediaMid"], info["mediaId"] = matched.MediaMID, matched.MediaMID, matched.MediaMID
	}
	if platform == "tx" {
		info["copyrightId"], info["contentId"] = matched.ID, matched.ID
	}
	return info
}

func (m *sourceManager) getResolver(script string) (*sourceplugin.Resolver, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.enabled || m.script != script {
		return nil, errors.New("播放源设置已更改")
	}
	if m.resolver != nil {
		return m.resolver, nil
	}
	resolver, _, err := sourceplugin.NewResolver(script)
	if err != nil {
		return nil, err
	}
	resolver.SetLogHandler(func(level, message string) { m.addLog("插件 "+level, message) })
	m.resolver = resolver
	return resolver, nil
}

func sourcePluginDir() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "source-plugins"), nil
}

func sourcePluginPath(name string) (string, error) {
	if name == "" || filepath.Base(name) != name || strings.ToLower(filepath.Ext(name)) != ".js" {
		return "", errors.New("invalid source plugin name")
	}
	dir, err := sourcePluginDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func readSourcePlugin(path string) ([]byte, error) {
	if strings.ToLower(filepath.Ext(path)) != ".js" {
		return nil, errors.New("请选择 .js 音源脚本")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() <= 0 || info.Size() > sourceplugin.MaxScriptSize {
		return nil, fmt.Errorf("音源脚本大小需小于 %d 字节", sourceplugin.MaxScriptSize)
	}
	return os.ReadFile(path)
}

func (a *app) loadSourcePlugin() {
	if a.settings.SourcePlugin == "" || a.source == nil {
		return
	}
	path, err := sourcePluginPath(a.settings.SourcePlugin)
	if err != nil {
		a.source.setFailure("脚本路径无效，请重新导入")
		return
	}
	data, err := readSourcePlugin(path)
	if err != nil {
		a.source.setFailure("脚本文件不存在，请重新导入")
		return
	}
	script := string(data)
	info, err := sourceplugin.Validate(script)
	if err != nil {
		a.source.setFailure("脚本校验失败，请重新导入")
		return
	}
	a.source.configure(script, info, a.settings.QQFallback)
}

var chooseSourcePluginFile = func(parent *mygo.Window) (string, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: parent, Title: "选择付费音源脚本", ButtonLabel: "导入",
		Filters: []mygo.FileFilter{{Name: "JavaScript 音源", Extensions: []string{"js"}}},
	})
	if err != nil || len(paths) == 0 {
		return "", err
	}
	return paths[0], nil
}

func (a *app) importSourcePlugin() {
	win := a.win
	go func() {
		path, err := chooseSourcePluginFile(win)
		if err != nil || path == "" {
			return
		}
		data, err := readSourcePlugin(path)
		if err != nil {
			a.post(func() { a.sourcePluginError(err) })
			return
		}
		script := string(data)
		info, err := sourceplugin.Validate(script)
		if err != nil {
			a.post(func() { a.sourcePluginError(err) })
			return
		}
		dir, err := sourcePluginDir()
		if err != nil {
			a.post(func() { a.sourcePluginError(err) })
			return
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			a.post(func() { a.sourcePluginError(err) })
			return
		}
		sum := sha256.Sum256(data)
		name := "source-" + hex.EncodeToString(sum[:8]) + ".js"
		target := filepath.Join(dir, name)
		if err := writeFileAtomic(target, data, 0o600); err != nil {
			a.post(func() { a.sourcePluginError(err) })
			return
		}
		a.post(func() {
			if a.source == nil {
				a.source = newSourceManager()
			}
			a.source.configure(script, info, true)
			a.settings.SourcePlugin = name
			a.settings.QQFallback = true
			a.changed()
			a.toast = "已添加音源插件：" + info.Name
			a.redraw()
		})
	}()
}

func (a *app) testSourcePlugin() {
	manager := a.source
	if manager == nil || !manager.hasPlugin() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		track := netease.Track{
			Name: "晴天", Duration: 269000,
			Artists: []netease.Artist{{Name: "周杰伦"}},
			Album:   netease.Album{Name: "叶惠美"},
		}
		// Paid source scripts use their own quality names, not NetEase levels.
		stream, err := manager.Resolve(ctx, track, "higher", nil)
		message := "播放源解析测试失败，请检查插件和服务状态"
		if err != nil {
			message = "播放源解析测试失败：" + err.Error()
		}
		if err == nil && stream != nil && stream.URL != "" {
			message = "播放源解析测试通过"
		}
		a.post(func() {
			a.toast = message
			a.redraw()
		})
	}()
}

func (a *app) sourcePluginError(err error) {
	if err == nil {
		return
	}
	a.toast = "无法导入音源插件：" + err.Error()
	a.redraw()
}

func (a *app) sourcePluginLabel() string {
	if a.settings.SourcePlugin == "" {
		return "尚未添加"
	}
	if a.source == nil {
		return "脚本未加载，请重新导入"
	}
	return a.source.name()
}

func (m *sourceManager) name() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.info.Name == "" {
		if m.failure != "" {
			return m.failure
		}
		return "JavaScript 音源"
	}
	return m.info.Name
}
