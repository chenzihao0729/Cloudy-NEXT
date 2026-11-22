package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

func (a *app) sourceLogsDialog(c *ui.Context) {
	v := &a.settingsView
	if !v.sourceLogs {
		return
	}
	w, h := c.Size()
	ink := a.inkOn(false)
	entries := []sourceLogEntry(nil)
	if a.source != nil {
		entries = a.source.logEntries()
	}
	a.dialog(c, "source-plugin-logs", 0.35, &v.sourceLogs, func(panel ui.Element) {
		panel.Width(min(760, w-64)).Height(min(620, h-64)).TextColor(ink.text)
		a.pane(panel, 20, false)
		panel.Children(func() {
			dialogHeader(c, ink, "播放源运行日志", fmt.Sprintf("最近 %d 条", len(entries)), func() { v.sourceLogs = false })
			fill, edge := a.cardColors()
			list := ui.Scroll(c.Key("source-plugin-log-list")).TrackScroll(&v.sourceLogScroll).Grow(1).MinHeight(0).Margin(0, 24).Padding(12).Radius(14).Background(fill).Border(0.5, edge).Clip()
			list.Children(func() {
				if len(entries) == 0 {
					ui.Text(c, "暂无运行记录").FontSize(13).TextColor(ink.muted).Margin(8)
					return
				}
				for i := len(entries) - 1; i >= 0; i-- {
					entry := entries[i]
					color := ink.muted
					if entry.level == "error" || entry.level == "插件 error" {
						color = dangerColor
					}
					ui.Text(c.Key(i), entry.at.Format("15:04:05")+"  ["+entry.level+"]  "+entry.message).
						FontSize(11.5).Font("Cascadia Mono, Consolas, monospace").TextColor(color).LineHeight(1.35).Margin(4, 2).Selectable()
				}
			})
			dialogFooter(c, func() {
				if settingButton(c.Key("source-plugin-log-clear"), ink, "清空", iconTrash, len(entries) == 0, true).Clicked() && a.source != nil {
					a.source.clearLogs()
					a.redraw()
				}
				if settingButton(c.Key("source-plugin-log-close"), ink, "关闭", iconX, false, false).Clicked() {
					v.sourceLogs = false
				}
			})
		})
	})
}
