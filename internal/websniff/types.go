package websniff

type HAR struct {
	Log HARLog `json:"log"`
}

type HARLog struct {
	Entries []HAREntry `json:"entries"`
}

type HAREntry struct {
	Request  HARRequest  `json:"request"`
	Response HARResponse `json:"response"`
}

type HARRequest struct {
	Method   string       `json:"method"`
	URL      string       `json:"url"`
	Headers  []HARHeader  `json:"headers"`
	PostData *HARPostData `json:"postData,omitempty"`
}

type HARPostData struct {
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

type HARResponse struct {
	Status  int                `json:"status"`
	Content HARResponseContent `json:"content"`
}

type HARResponseContent struct {
	MimeType string `json:"mimeType"`
	Size     int    `json:"size"`
	Text     string `json:"text"`
}

type HARHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type EnrichedCapture struct {
	TargetURL         string          `json:"target_url"`
	CapturedAt        string          `json:"captured_at"`
	InteractionRounds int             `json:"interaction_rounds"`
	Auth              *AuthCapture    `json:"auth,omitempty"`
	Entries           []EnrichedEntry `json:"entries"`
}

type AuthCapture struct {
	Headers     map[string]string `json:"headers"`
	Cookies     []string          `json:"cookies"`
	Type        string            `json:"type"`
	BoundDomain string            `json:"bound_domain"`
	ExpiresAt   string            `json:"expires_at"`
}

type EnrichedEntry struct {
	Method              string            `json:"method"`
	URL                 string            `json:"url"`
	RequestBody         string            `json:"request_body"`
	ResponseBody        string            `json:"response_body"`
	ResponseStatus      int               `json:"response_status"`
	ResponseContentType string            `json:"response_content_type"`
	RequestHeaders      map[string]string `json:"request_headers"`
	Classification      string            `json:"classification"`
	IsNoise             bool              `json:"is_noise"`
}
