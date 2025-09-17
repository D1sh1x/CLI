package main

type GrepFlags struct {
	Regex      bool // -E (по умолчанию true)
	Fixed      bool // -F
	IgnoreCase bool // -i
	Invert     bool // -v
	MaxCount   int  // -m
	CountOnly  bool // -c
}

type GrepRequest struct {
	JobID   string    `json:"job_id"`
	Pattern string    `json:"pattern"`
	Flags   GrepFlags `json:"flags"`
	Lines   []string  `json:"lines,omitempty"`
	Workers int       `json:"workers,omitempty"`
}

type GrepResponse struct {
	JobID   string   `json:"job_id"`
	Node    string   `json:"node"`
	Matches []string `json:"matches,omitempty"`
	Count   int      `json:"count"`
	Error   string   `json:"error,omitempty"`
}
