package index

import "time"

type FileInfo struct {
	Path    string `json:"path"`
	Lang    string `json:"lang"`
	Size    int64  `json:"size"`
	Lines   int    `json:"lines"`
	Package string `json:"package,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type Symbol struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"` 
	File     string `json:"file"`
	Line     int    `json:"line"`
	Doc      string `json:"doc,omitempty"`
	Receiver string `json:"receiver,omitempty"` 
}

type Index struct {
	Root    string     `json:"root"`
	Files   []FileInfo `json:"files"`
	Symbols []Symbol   `json:"symbols"`
	Dirs    []string   `json:"dirs"`
	BuiltAt time.Time  `json:"built_at"`
}
