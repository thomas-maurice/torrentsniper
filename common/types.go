package common

type Torrent struct {
	Link     string            `json:"torrent_link"`
	Source   string            `json:"torrent_source"`
	Category string            `json:"torrent_category"`
	Type     string            `json:"torrent_type"`
	Data     string            `json:"torrent_data,omitempty"`
	Name     string            `json:"torrent_name,omitempty"`
	Hash     string            `json:"torrent_hash,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type HTTPPostedTorrent struct {
	EncodedValue string            `json:"encoded,omitempty"`
	Source       string            `json:"torrent_source,omitempty"`
	Category     string            `json:"torrent_category,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type Peer struct {
	IP string `json:"ip"`
	Torrent
	SeenAt int64 `json:"seen_at"`
}
