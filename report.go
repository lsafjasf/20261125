package main

// MediaLocation identifies a picture or sound interval affected by a change.
type MediaLocation struct {
	TrackID        uint32  `json:"track_id,omitempty"`
	TrackKind      string  `json:"track_kind,omitempty"`
	SampleIndex    int     `json:"sample_index,omitempty"`
	DisplayOrder   int     `json:"display_order,omitempty"`
	Sync           bool    `json:"sync,omitempty"`
	PTS            int64   `json:"pts,omitempty"`
	DTS            int64   `json:"dts,omitempty"`
	StartTime      float64 `json:"start_seconds,omitempty"`
	EndTime        float64 `json:"end_seconds,omitempty"`
	IntervalIndex  int     `json:"interval_index,omitempty"`
	AudioStart     int64   `json:"audio_start_sample,omitempty"`
	AudioEnd       int64   `json:"audio_end_sample,omitempty"`
	EarliestSample int64   `json:"earliest_sample,omitempty"`
	Description    string  `json:"description,omitempty"`
}

// Change is one semantic or physical difference between two archives.
type Change struct {
	Layer          string         `json:"layer"`
	Kind           string         `json:"kind"`
	Severity       string         `json:"severity"` // media, container, ignored
	Message        string         `json:"message,omitempty"`
	OriginalOffset int64          `json:"original_offset"`
	CopyOffset     int64          `json:"copy_offset"`
	RelativeOffset int64          `json:"relative_offset,omitempty"`
	OriginalEnd    int64          `json:"original_end,omitempty"`
	CopyEnd        int64          `json:"copy_end,omitempty"`
	OriginalHash   string         `json:"original_hash,omitempty"`
	CopyHash       string         `json:"copy_hash,omitempty"`
	BlockIndex     int            `json:"block_index,omitempty"`
	ChunkIndex     int            `json:"chunk_index,omitempty"`
	BoxPath        string         `json:"box_path,omitempty"`
	MetadataOnly   bool           `json:"metadata_only,omitempty"`
	Media          *MediaLocation `json:"media,omitempty"`
}

// Report is the result of comparing two layered manifests.
type Report struct {
	Schema              string   `json:"schema"`
	OriginalFile        string   `json:"original_file,omitempty"`
	CopyFile            string   `json:"copy_file,omitempty"`
	Format              string   `json:"format"`
	WholeFileIdentical  bool     `json:"whole_file_identical"`
	MediaIdentical      bool     `json:"media_identical"`
	ContainerIdentical  bool     `json:"container_identical"`
	NormalRearrangement bool     `json:"normal_rearrangement,omitempty"`
	EarliestByte        *Change  `json:"earliest_byte,omitempty"`
	EarliestMediaChange *Change  `json:"earliest_media_change,omitempty"`
	Changes             []Change `json:"changes,omitempty"`
	ContainerChanges    []Change `json:"container_changes,omitempty"`
	IgnoredChanges      []Change `json:"ignored_changes,omitempty"`
	Rearrangements      []Change `json:"rearrangements,omitempty"`
	Warnings            []string `json:"warnings,omitempty"`
}

func (r *Report) addMedia(c Change) {
	c.Severity = "media"
	r.Changes = append(r.Changes, c)
	if r.EarliestMediaChange == nil {
		cp := c
		r.EarliestMediaChange = &cp
	}
}

func (r *Report) addContainer(c Change) {
	if c.Severity == "" {
		c.Severity = "container"
	}
	r.ContainerChanges = append(r.ContainerChanges, c)
}

func (r *Report) addIgnored(c Change) {
	c.Severity = "ignored"
	r.IgnoredChanges = append(r.IgnoredChanges, c)
}

func (r *Report) addRearrangement(c Change) {
	c.Severity = "rearrangement"
	r.Rearrangements = append(r.Rearrangements, c)
}
