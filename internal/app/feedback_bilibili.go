package app

import (
	"fmt"
	"net/url"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
)

// feedbackMessage is what a listener sends: what they wrote, and what the
// author needs to know to look into it.
func feedbackMessage(text string) string {
	return fmt.Sprintf("%s\n\n——\nCloudy-NEXT %s · %s · %s", strings.TrimSpace(text), mygo.App.Version(), osVersion(), runtime.GOARCH)
}

// feedbackPage is the author's private messages on Bilibili.
func feedbackPage() string {
	return fmt.Sprintf("https://message.bilibili.com/#/whisper/mid%d", feedbackBilibiliUID)
}

// openURL opens a page in the browser; tests replace it.
var openURL = mygo.Shell.OpenExternal

// feedbackToBilibili puts the message on the clipboard and opens the
// author's private messages, for the listener to paste it there: Bilibili
// lets no app send one, and its messages reach the author wherever the
// listener is, as mail through a form did not.
func (a *app) feedbackToBilibili() {
	f := &a.feedback
	if strings.TrimSpace(f.text) == "" {
		return
	}
	if err := copyText(feedbackMessage(f.text)); err != nil {
		f.note, f.failed = "无法复制到剪贴板，请手动复制上面的内容", true
		return
	}
	if err := openURL(feedbackPage()); err != nil {
		f.note, f.failed = "已复制，但无法打开浏览器："+feedbackPage(), true
		return
	}
	f.note, f.failed = "已复制反馈内容，在打开的 B 站私信里粘贴发送即可；图片请在私信里添加", false
	a.toast = "反馈内容已复制"
}

// feedbackIntro says where what is written goes.
// feedbackMailOpen says whether feedback can go by mail. It cannot for now:
// the mail is off until it is mended, and the Bilibili messages are the way.
var feedbackMailOpen = false

func feedbackIntro() string {
	if feedbackBilibiliUID == 0 {
		return "问题、想法或建议都可以写在这里，会连同图片和软件版本一起发给nya"
	}
	return "问题、想法或建议都可以写在这里。「去 Issue 反馈」会把内容和软件、系统版本填进 GitHub 的新 issue，登录 GitHub 后提交即可；没有 GitHub 账号，可以「发到 B 站私信」，会复制内容并打开 nya 的私信页面，粘贴发送"
}

// feedbackIssues is where issues are written.
const feedbackIssues = "https://github.com/" + releaseRepo + "/issues/new"

// feedbackIssueMost is the longest address the issue goes in: browsers
// and GitHub take about 8 KB of one.
const feedbackIssueMost = 7000

// feedbackIssueURL is the new issue, written: titled by the first line,
// what was written and the versions in its body. Nothing written, it is
// the templates to choose from. long says that the body did not fit and
// is left out, to paste.
func feedbackIssueURL(text string) (link string, long bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return feedbackIssues + "/choose", false
	}
	title := []rune(strings.TrimSpace(strings.SplitN(text, "\n", 2)[0]))
	if len(title) > 40 {
		title = append(title[:40], '…')
	}
	q := url.Values{"title": {string(title)}, "body": {feedbackMessage(text)}}
	link = feedbackIssues + "?" + q.Encode()
	if len(link) <= feedbackIssueMost {
		return link, false
	}
	return feedbackIssues + "?" + url.Values{"title": {string(title)}}.Encode(), true
}

// feedbackToIssue opens a new issue on GitHub, written with what is in
// the box: a long message goes on the clipboard instead, to paste there.
func (a *app) feedbackToIssue() {
	f := &a.feedback
	link, long := feedbackIssueURL(f.text)
	if long {
		if err := copyText(feedbackMessage(f.text)); err != nil {
			f.note, f.failed = "内容太长，也没能复制，请手动复制上面的内容", true
			return
		}
	}
	if err := openURL(link); err != nil {
		f.note, f.failed = "无法打开浏览器："+feedbackIssues, true
		return
	}
	switch {
	case strings.TrimSpace(f.text) == "":
		f.note = "已打开 GitHub，选一个模板写下你的反馈"
	case long:
		f.note = "内容较长，已复制到剪贴板，在打开的 issue 里粘贴即可"
	default:
		f.note = "已打开 GitHub，内容已填好，登录后提交即可；图片可以直接粘贴到 issue 里"
	}
	f.failed = false
}
