package weixin

// BaseInfo is common request metadata.
type BaseInfo struct {
	ChannelVersion string `json:"channel_version,omitempty"`
}

// MessageType constants.
const (
	MsgTypeNone = 0
	MsgTypeUser = 1
	MsgTypeBot  = 2
)

// MessageItemType constants.
const (
	ItemTypeNone  = 0
	ItemTypeText  = 1
	ItemTypeImage = 2
	ItemTypeVoice = 3
	ItemTypeFile  = 4
	ItemTypeVideo = 5
)

// MessageState constants.
const (
	StateNew        = 0
	StateGenerating = 1
	StateFinish     = 2
)

// TextItem is a text message item.
type TextItem struct {
	Text string `json:"text,omitempty"`
}

// CDNMedia is a CDN media reference.
type CDNMedia struct {
	EncryptQueryParam string `json:"encrypt_query_param,omitempty"`
	AESKey            string `json:"aes_key,omitempty"`
	EncryptType       int    `json:"encrypt_type,omitempty"`
	FullURL           string `json:"full_url,omitempty"`
}

// ImageItem is an image message item.
type ImageItem struct {
	Media      *CDNMedia `json:"media,omitempty"`
	ThumbMedia *CDNMedia `json:"thumb_media,omitempty"`
	AESKey     string    `json:"aeskey,omitempty"`
	URL        string    `json:"url,omitempty"`
}

// VoiceItem is a voice message item.
type VoiceItem struct {
	Media     *CDNMedia `json:"media,omitempty"`
	Text      string    `json:"text,omitempty"`
	Playtime  int       `json:"playtime,omitempty"`
}

// FileItem is a file message item.
type FileItem struct {
	Media    *CDNMedia `json:"media,omitempty"`
	FileName string    `json:"file_name,omitempty"`
	MD5      string    `json:"md5,omitempty"`
}

// VideoItem is a video message item.
type VideoItem struct {
	Media      *CDNMedia `json:"media,omitempty"`
	ThumbMedia *CDNMedia `json:"thumb_media,omitempty"`
	PlayLength int       `json:"play_length,omitempty"`
}

// MessageItem is a single item within a message.
type MessageItem struct {
	Type         int    `json:"type,omitempty"`
	MsgID        string `json:"msg_id,omitempty"`
	TextItem     *TextItem  `json:"text_item,omitempty"`
	ImageItem    *ImageItem `json:"image_item,omitempty"`
	VoiceItem    *VoiceItem `json:"voice_item,omitempty"`
	FileItem     *FileItem  `json:"file_item,omitempty"`
	VideoItem    *VideoItem `json:"video_item,omitempty"`
}

// WeixinMessage is a unified message from/to WeChat.
type WeixinMessage struct {
	Seq          int            `json:"seq,omitempty"`
	MessageID    int64          `json:"message_id,omitempty"`
	FromUserID   string         `json:"from_user_id,omitempty"`
	ToUserID     string         `json:"to_user_id,omitempty"`
	ClientID     string         `json:"client_id,omitempty"`
	CreateTimeMs int64          `json:"create_time_ms,omitempty"`
	UpdateTimeMs int64          `json:"update_time_ms,omitempty"`
	SessionID    string         `json:"session_id,omitempty"`
	GroupID      string         `json:"group_id,omitempty"`
	MessageType  int            `json:"message_type,omitempty"`
	MessageState int            `json:"message_state,omitempty"`
	ItemList     []MessageItem  `json:"item_list,omitempty"`
	ContextToken string         `json:"context_token,omitempty"`
}

// GetUpdatesReq is a request for new messages.
type GetUpdatesReq struct {
	GetUpdatesBuf string `json:"get_updates_buf"`
}

// GetUpdatesResp is the response containing new messages.
type GetUpdatesResp struct {
	Ret                int              `json:"ret,omitempty"`
	ErrCode            int              `json:"errcode,omitempty"`
	ErrMsg             string           `json:"errmsg,omitempty"`
	Msgs               []WeixinMessage  `json:"msgs,omitempty"`
	GetUpdatesBuf      string           `json:"get_updates_buf,omitempty"`
	LongPollTimeoutMs  int              `json:"longpolling_timeout_ms,omitempty"`
}

// SendMessageReq wraps a single message for sending.
type SendMessageReq struct {
	Msg *WeixinMessage `json:"msg,omitempty"`
}

// SendMessageResp is the send response (empty on success).
type SendMessageResp struct{}

// NotifyStartReq notifies the server when the channel starts.
type NotifyStartReq struct {
	BaseInfo *BaseInfo `json:"base_info,omitempty"`
}

// NotifyStartResp is the notify-start response.
type NotifyStartResp struct {
	Ret   int    `json:"ret,omitempty"`
	ErrMsg string `json:"errmsg,omitempty"`
}

// NotifyStopReq notifies the server when the channel stops.
type NotifyStopReq struct {
	BaseInfo *BaseInfo `json:"base_info,omitempty"`
}

// NotifyStopResp is the notify-stop response.
type NotifyStopResp struct {
	Ret   int    `json:"ret,omitempty"`
	ErrMsg string `json:"errmsg,omitempty"`
}

// QRCodeResponse is the response from get_bot_qrcode.
type QRCodeResponse struct {
	QRCode          string `json:"qrcode"`
	QRCodeImgContent string `json:"qrcode_img_content"`
}

// QRStatusResponse is the response from get_qrcode_status.
type QRStatusResponse struct {
	Status       string `json:"status"`
	BotToken     string `json:"bot_token,omitempty"`
	ILinkBotID   string `json:"ilink_bot_id,omitempty"`
	BaseURL      string `json:"baseurl,omitempty"`
	ILinkUserID  string `json:"ilink_user_id,omitempty"`
	RedirectHost string `json:"redirect_host,omitempty"`
}

// AccountData is the persistent account credential.
type AccountData struct {
	Token      string `json:"token,omitempty"`
	BaseURL    string `json:"baseUrl,omitempty"`
	UserID     string `json:"userId,omitempty"`
	ChatUserID string `json:"chatUserId,omitempty"`
	SavedAt    string `json:"savedAt,omitempty"`
}
