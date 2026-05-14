package messages

import (
	"encoding/xml"
	"fmt"
	"strings"
)

const (
	messageTypeText      = 1
	messageTypeImage     = 3
	messageTypeVoice     = 34
	messageTypeCard      = 42
	messageTypeVideo     = 43
	messageTypeAnimation = 47
	messageTypeLocation  = 48
	messageTypeShare     = 49
	messageTypeVOIP      = 50
	messageTypeSystem    = 10000

	messageSubTypeText             = 1
	messageSubTypeLink             = 4
	messageSubTypeLink2            = 5
	messageSubTypeFile             = 6
	messageSubTypeGIF              = 8
	messageSubTypeMiniProgram      = 33
	messageSubTypeMiniProgram2     = 36
	messageSubTypeChannel          = 51
	messageSubTypeQuote            = 57
	messageSubTypePat              = 62
	messageSubTypeChannelLive      = 63
	messageSubTypeMusic            = 92
	messageSubTypePay              = 2000
	messageSubTypeRedEnvelope      = 2001
	messageSubTypeRedEnvelopeCover = 2003
)

type normalizedContent struct {
	Type    string
	Text    string
	Details map[string]string
}

func (c normalizedContent) Detail() map[string]string {
	detail := make(map[string]string, len(c.Details)+2)
	if c.Type != "" {
		detail["type"] = c.Type
	}
	if c.Text != "" {
		detail["text"] = c.Text
	}
	for key, value := range c.Details {
		if value != "" {
			detail[key] = value
		}
	}
	if len(detail) == 0 {
		return nil
	}
	return detail
}

type mediaMsg struct {
	XMLName  xml.Name    `xml:"msg"`
	App      appMsg      `xml:"appmsg"`
	Image    imageMsg    `xml:"img"`
	Location locationMsg `xml:"location"`
	Emoji    emojiMsg    `xml:"emoji"`
}

type appMsg struct {
	Type              int         `xml:"type"`
	Title             string      `xml:"title"`
	Des               string      `xml:"des"`
	URL               string      `xml:"url"`
	MD5               string      `xml:"md5"`
	SourceUsername    string      `xml:"sourceusername"`
	SourceDisplayName string      `xml:"sourcedisplayname"`
	AppAttach         *appAttach  `xml:"appattach"`
	FinderFeed        *finderFeed `xml:"finderFeed"`
	FinderLive        *finderLive `xml:"finderLive"`
	WCPayInfo         *wcPayInfo  `xml:"wcpayinfo"`
}

type appAttach struct {
	FileExt  string `xml:"fileext"`
	TotalLen string `xml:"totallen"`
}

type imageMsg struct {
	CDNThumbURL    string `xml:"cdnthumburl,attr"`
	CDNThumbLength string `xml:"cdnthumblength,attr"`
	CDNThumbWidth  string `xml:"cdnthumbwidth,attr"`
	CDNThumbHeight string `xml:"cdnthumbheight,attr"`
	CDNMidImgURL   string `xml:"cdnmidimgurl,attr"`
	CDNMidWidth    string `xml:"cdnmidwidth,attr"`
	CDNMidHeight   string `xml:"cdnmidheight,attr"`
	CDNHDWidth     string `xml:"cdnhdwidth,attr"`
	CDNHDHeight    string `xml:"cdnhdheight,attr"`
	Length         string `xml:"length,attr"`
	MD5            string `xml:"md5,attr"`
	HEVCMidSize    string `xml:"hevc_mid_size,attr"`
	OriginMD5      string `xml:"originsourcemd5,attr"`
}

type finderFeed struct {
	Desc      string          `xml:"desc"`
	Nickname  string          `xml:"nickname"`
	MediaList finderMediaList `xml:"mediaList"`
}

type finderMediaList struct {
	Media []finderMedia `xml:"media"`
}

type finderMedia struct {
	URL string `xml:"url"`
}

type finderLive struct {
	Desc string `xml:"desc"`
}

type wcPayInfo struct {
	PaySubType int    `xml:"paysubtype"`
	FeeDesc    string `xml:"feedesc"`
	PayMemo    string `xml:"pay_memo"`
}

type locationMsg struct {
	X        string `xml:"x,attr"`
	Y        string `xml:"y,attr"`
	Label    string `xml:"label,attr"`
	CityName string `xml:"cityname,attr"`
}

type emojiMsg struct {
	CdnURL string `xml:"cdnurl,attr"`
}

func normalizeContent(msgType int64, subType int64, raw string) normalizedContent {
	switch msgType {
	case messageTypeText:
		return normalizedContent{Type: "text", Text: raw}
	case messageTypeImage:
		return normalizeImage(raw)
	case messageTypeVoice:
		return normalizedContent{Type: "voice", Text: "[语音]"}
	case messageTypeCard:
		return normalizedContent{Type: "card", Text: "[名片]"}
	case messageTypeVideo:
		return normalizedContent{Type: "video", Text: "[视频]"}
	case messageTypeAnimation:
		if msg, ok := parseMedia(raw); ok && msg.Emoji.CdnURL != "" {
			return normalizedContent{Type: "emoji", Text: "[动画表情]", Details: map[string]string{"url": msg.Emoji.CdnURL}}
		}
		return normalizedContent{Type: "emoji", Text: "[动画表情]"}
	case messageTypeLocation:
		return normalizeLocation(raw)
	case messageTypeShare:
		return normalizeShare(subType, raw)
	case messageTypeVOIP:
		return normalizedContent{Type: "voip", Text: "[语音通话]"}
	case messageTypeSystem:
		return normalizedContent{Type: "system", Text: strings.TrimSpace(raw)}
	default:
		return normalizedContent{Type: "unknown", Text: strings.TrimSpace(raw)}
	}
}

func normalizeImage(raw string) normalizedContent {
	msg, ok := parseMedia(raw)
	if !ok {
		return normalizedContent{Type: "image", Text: "[图片]"}
	}
	img := msg.Image
	details := map[string]string{}
	putIfNotEmpty(details, "md5", img.MD5)
	putIfNotEmpty(details, "origin_md5", img.OriginMD5)
	putIfNonZero(details, "length", img.Length)
	putIfNotEmpty(details, "cdn_thumb_url", img.CDNThumbURL)
	putIfNonZero(details, "cdn_thumb_length", img.CDNThumbLength)
	putIfNonZero(details, "cdn_thumb_width", img.CDNThumbWidth)
	putIfNonZero(details, "cdn_thumb_height", img.CDNThumbHeight)
	putIfNotEmpty(details, "cdn_mid_img_url", img.CDNMidImgURL)
	putIfNonZero(details, "cdn_mid_width", img.CDNMidWidth)
	putIfNonZero(details, "cdn_mid_height", img.CDNMidHeight)
	putIfNonZero(details, "cdn_hd_width", img.CDNHDWidth)
	putIfNonZero(details, "cdn_hd_height", img.CDNHDHeight)
	putIfNonZero(details, "hevc_mid_size", img.HEVCMidSize)
	return normalizedContent{Type: "image", Text: "[图片]", Details: detailsOrNil(details)}
}

func normalizeLocation(raw string) normalizedContent {
	msg, ok := parseMedia(raw)
	if !ok {
		return normalizedContent{Type: "location", Text: "[位置]"}
	}
	parts := compactStrings(msg.Location.Label, msg.Location.CityName, msg.Location.X, msg.Location.Y)
	details := map[string]string{}
	putIfNotEmpty(details, "label", msg.Location.Label)
	putIfNotEmpty(details, "city", msg.Location.CityName)
	putIfNotEmpty(details, "x", msg.Location.X)
	putIfNotEmpty(details, "y", msg.Location.Y)
	if len(parts) == 0 {
		return normalizedContent{Type: "location", Text: "[位置]", Details: detailsOrNil(details)}
	}
	return normalizedContent{Type: "location", Text: fmt.Sprintf("[位置|%s]", strings.Join(parts, "|")), Details: detailsOrNil(details)}
}

func normalizeShare(subType int64, raw string) normalizedContent {
	msg, ok := parseMedia(raw)
	if !ok {
		return normalizedContent{Type: "share", Text: strings.TrimSpace(raw)}
	}
	app := msg.App
	appType := int64(app.Type)
	if appType == 0 {
		appType = subType
	}
	details := map[string]string{"app_type": fmt.Sprintf("%d", appType)}
	putIfNotEmpty(details, "title", strings.TrimSpace(app.Title))
	putIfNotEmpty(details, "desc", strings.TrimSpace(app.Des))
	putIfNotEmpty(details, "url", strings.TrimSpace(app.URL))
	putIfNotEmpty(details, "source_username", strings.TrimSpace(app.SourceUsername))
	putIfNotEmpty(details, "source_display_name", strings.TrimSpace(app.SourceDisplayName))

	switch appType {
	case messageSubTypeText, messageSubTypeLink, messageSubTypeLink2:
		return normalizedContent{Type: "link", Text: linkText(app.Title, app.Des, app.URL), Details: detailsOrNil(details)}
	case messageSubTypeFile:
		putIfNotEmpty(details, "md5", app.MD5)
		if app.AppAttach != nil {
			putIfNotEmpty(details, "file_ext", app.AppAttach.FileExt)
			putIfNotEmpty(details, "total_len", app.AppAttach.TotalLen)
		}
		return normalizedContent{Type: "file", Text: bracketText("文件", app.Title), Details: detailsOrNil(details)}
	case messageSubTypeGIF:
		if app.URL != "" {
			putIfNotEmpty(details, "url", app.URL)
		} else if msg.Emoji.CdnURL != "" {
			putIfNotEmpty(details, "url", msg.Emoji.CdnURL)
		}
		return normalizedContent{Type: "gif", Text: "[GIF表情]", Details: detailsOrNil(details)}
	case messageSubTypeMiniProgram, messageSubTypeMiniProgram2:
		title := firstNonEmpty(app.SourceDisplayName, app.Title)
		return normalizedContent{Type: "mini_program", Text: linkLikeText("小程序", title, app.URL), Details: detailsOrNil(details)}
	case messageSubTypeChannel:
		if app.FinderFeed != nil {
			title := strings.TrimSpace(strings.ReplaceAll(app.FinderFeed.Desc, "\n", " "))
			url := ""
			if len(app.FinderFeed.MediaList.Media) > 0 {
				url = app.FinderFeed.MediaList.Media[0].URL
			}
			putIfNotEmpty(details, "title", title)
			putIfNotEmpty(details, "url", url)
			putIfNotEmpty(details, "nickname", app.FinderFeed.Nickname)
			return normalizedContent{Type: "channel", Text: linkLikeText("视频号", title, url), Details: detailsOrNil(details)}
		}
		return normalizedContent{Type: "channel", Text: bracketText("视频号", app.Title), Details: detailsOrNil(details)}
	case messageSubTypeQuote:
		return normalizedContent{Type: "quote", Text: quoteText(app.Title), Details: detailsOrNil(details)}
	case messageSubTypePat:
		return normalizedContent{Type: "pat", Text: firstNonEmpty(app.Title, "[拍一拍]"), Details: detailsOrNil(details)}
	case messageSubTypeChannelLive:
		title := app.Title
		if app.FinderLive != nil && app.FinderLive.Desc != "" {
			title = app.FinderLive.Desc
		}
		return normalizedContent{Type: "channel_live", Text: bracketText("视频号直播", title), Details: detailsOrNil(details)}
	case messageSubTypeMusic:
		return normalizedContent{Type: "music", Text: linkLikeText("音乐", app.Title, app.URL), Details: detailsOrNil(details)}
	case messageSubTypePay:
		return normalizedContent{Type: "pay", Text: payText(app.WCPayInfo), Details: detailsOrNil(details)}
	case messageSubTypeRedEnvelope:
		return normalizedContent{Type: "red_envelope", Text: "[红包]", Details: detailsOrNil(details)}
	case messageSubTypeRedEnvelopeCover:
		return normalizedContent{Type: "red_envelope_cover", Text: "[红包封面]", Details: detailsOrNil(details)}
	default:
		return normalizedContent{Type: "share", Text: bracketText("分享", app.Title), Details: detailsOrNil(details)}
	}
}

func parseMedia(raw string) (mediaMsg, bool) {
	var msg mediaMsg
	if err := xml.Unmarshal([]byte(raw), &msg); err != nil {
		return mediaMsg{}, false
	}
	return msg, true
}

func linkText(title string, desc string, rawURL string) string {
	title = firstNonEmpty(strings.TrimSpace(title), "链接")
	desc = strings.TrimSpace(desc)
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" && strings.HasPrefix(desc, "http") {
		rawURL = desc
	}
	text := linkLikeText("链接", title, rawURL)
	if desc != "" && desc != rawURL {
		text += "\n" + desc
	}
	return text
}

func linkLikeText(kind string, title string, rawURL string) string {
	title = strings.TrimSpace(title)
	rawURL = strings.TrimSpace(rawURL)
	if title == "" {
		return "[" + kind + "]"
	}
	if rawURL == "" {
		return bracketText(kind, title)
	}
	return fmt.Sprintf("[%s|%s](%s)", kind, title, rawURL)
}

func bracketText(kind string, title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "[" + kind + "]"
	}
	return fmt.Sprintf("[%s|%s]", kind, title)
}

func quoteText(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "[引用]"
	}
	return "> [引用]\n" + title
}

func payText(info *wcPayInfo) string {
	if info == nil {
		return "[转账]"
	}
	action := ""
	switch info.PaySubType {
	case 1, 7:
		action = "发送 "
	case 3, 5:
		action = "接收 "
	case 4:
		action = "退还 "
	}
	memo := ""
	if info.PayMemo != "" {
		memo = "(" + info.PayMemo + ")"
	}
	if info.FeeDesc == "" {
		return "[转账]"
	}
	return fmt.Sprintf("[转账|%s%s]%s", action, info.FeeDesc, memo)
}

func compactStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func putIfNotEmpty(m map[string]string, key string, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		m[key] = value
	}
}

func putIfNonZero(m map[string]string, key string, value string) {
	value = strings.TrimSpace(value)
	if value != "" && value != "0" {
		m[key] = value
	}
}

func detailsOrNil(details map[string]string) map[string]string {
	if len(details) == 0 {
		return nil
	}
	return details
}
