package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Report is the top-level diagnosis document.
type Report struct {
	Tool        string            `json:"tool"`
	Input       string            `json:"input"`
	Reference   string            `json:"reference,omitempty"`
	GeneratedAt string            `json:"generated_at_utc"`
	Video       *VideoMeta        `json:"encoding_parameters,omitempty"`
	Config      Config            `json:"detector_config"`
	Summary     Summary           `json:"summary"`
	HotFrames   []HotFrame        `json:"hot_frames"`
	Frames      []FrameReportLite `json:"frames"`
}

// HotFrame is a frame selected for visual inspection, with zoom assets.
type HotFrame struct {
	Rank        int            `json:"rank"`
	FrameIndex  int            `json:"frame_index"`
	PTS         float64        `json:"pts_sec"`
	Score       float64        `json:"artifact_score"`
	IsKeyframe  bool           `json:"is_keyframe"`
	OverlayPNG  string         `json:"overlay_png"`
	Zooms       []Zoom         `json:"zooms"`
	LabelCounts map[string]int `json:"label_counts"`
}

type Zoom struct {
	Label   string `json:"label"`
	Region  Region `json:"region"`
	ZoomPNG string `json:"zoom_png"`
}

type FrameReportLite struct {
	Index    int            `json:"frame_index"`
	PTS      float64        `json:"pts_sec"`
	Score    float64        `json:"artifact_score"`
	Counts   map[string]int `json:"label_counts"`
	Hotspots []Region       `json:"hotspots"`
}

// Summary aggregates label prevalence over the whole sequence.
type Summary struct {
	FramesAnalyzed int            `json:"frames_analyzed"`
	TilesPerFrame  int            `json:"tiles_per_frame"`
	LabelTiles     map[string]int `json:"artifact_tile_hits"`
	LabelFrames    map[string]int `json:"frames_with_label"`
	WorstLabel     string         `json:"dominant_artifact"`
}

func buildReport(input, ref string, meta *VideoMeta, cfg Config,
	reports []FrameReport, keyframes map[int]bool, hot []HotFrame, generatedAt string) Report {
	r := Report{
		Tool: "artifactdiag (Go stdlib only)", Input: input, Reference: ref,
		GeneratedAt: generatedAt, Video: meta, Config: cfg,
		Summary: Summary{
			FramesAnalyzed: len(reports),
			TilesPerFrame:  len(reports[0].Tiles),
			LabelTiles:     map[string]int{},
			LabelFrames:    map[string]int{},
		},
		HotFrames: hot,
	}
	for _, fr := range reports {
		lite := FrameReportLite{Index: fr.Index, PTS: fr.PTS,
			Score: fr.Score, Counts: fr.Counts, Hotspots: fr.Hotspots}
		r.Frames = append(r.Frames, lite)
		seen := map[string]bool{}
		for _, t := range fr.Tiles {
			if !isArtifact(t.Label) {
				continue
			}
			r.Summary.LabelTiles[t.Label]++
			if !seen[t.Label] {
				r.Summary.LabelFrames[t.Label]++
				seen[t.Label] = true
			}
		}
	}
	r.Summary.WorstLabel = topKey(r.Summary.LabelTiles)
	return r
}

func topKey(m map[string]int) string {
	best, bestN := "", -1
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if m[k] > bestN {
			best, bestN = k, m[k]
		}
	}
	return best
}

func writeJSON(path string, r Report) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func writeCSV(path string, reports []FrameReport) error {
	var b strings.Builder
	b.WriteString("frame_index,pts_sec,artifact_score,blocking,ringing,banding_compression,banding_tone,motion,real_edge,text_edge\n")
	for _, fr := range reports {
		fmt.Fprintf(&b, "%d,%.3f,%.3f,%d,%d,%d,%d,%d,%d,%d\n",
			fr.Index, fr.PTS, fr.Score,
			fr.Counts[LabelBlocking], fr.Counts[LabelRinging],
			fr.Counts[LabelBandingComp], fr.Counts[LabelBandingTone],
			fr.Counts[LabelMotion], fr.Counts[LabelRealEdge],
			fr.Counts[LabelTextEdge])
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// writeViewer emits a self-contained HTML page for browsing hotspot zooms.
func writeViewer(path string, r Report) error {
	var b strings.Builder
	b.WriteString("<!doctype html><html><head><meta charset=utf-8>")
	b.WriteString("<title>artifactdiag report</title><style>")
	b.WriteString(viewerCSS)
	b.WriteString("</style></head><body><h1>Local artifact diagnosis</h1>")
	b.WriteString("<p>Input: <code>" + htmlEscape(r.Input) + "</code></p>")
	if r.Video != nil {
		v := r.Video
		fmt.Fprintf(&b, "<p>Codec: %s / %s, %dx%d, %.3f fps, %d frames, bitrate %d bps, B-frames=%v</p>",
			htmlEscape(v.Codec), htmlEscape(v.Profile), v.Width, v.Height,
			v.FrameRate, v.NbFrames, v.BitRate, v.HasBFrames)
	}
	b.WriteString("<h2>Legend</h2><ul class=legend>")
	for _, l := range []string{LabelBlocking, LabelRinging, LabelBandingComp,
		LabelBandingTone, LabelMotion, LabelRealEdge, LabelTextEdge} {
		rgb := labelColors[l]
		fmt.Fprintf(&b, "<li><span class=sw style='background:#%02x%02x%02x'></span>%s</li>",
			rgb[0], rgb[1], rgb[2], l)
	}
	b.WriteString("</ul>")
	b.WriteString("<div class=wrap><div><h2>Hot frames</h2><ol id=list></ol></div>")
	b.WriteString("<div><h2>Viewer</h2>")
	b.WriteString("<img id=img alt='zoom'><p id=cap></p></div></div>")
	data, _ := json.Marshal(r.HotFrames)
	b.WriteString("<script>const HOT=")
	b.Write(data)
	b.WriteString(viewerJS)
	b.WriteString("</script></body></html>")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;")
	return r.Replace(s)
}

const viewerCSS = `
body{font:14px/1.5 system-ui,sans-serif;margin:24px;color:#222}
code{background:#f2f2f2;padding:2px 5px;border-radius:4px}
.legend{display:flex;gap:14px;list-style:none;padding:0;flex-wrap:wrap}
.sw{display:inline-block;width:14px;height:14px;border-radius:3px;margin-right:5px;vertical-align:-2px}
.wrap{display:grid;grid-template-columns:280px 1fr;gap:24px}
#list li{cursor:pointer;padding:6px 8px;border-radius:6px}
#list li.sel{background:#e6f0ff}
#img{max-width:100%;border:1px solid #ccc;cursor:zoom-in;image-rendering:pixelated}
button{margin:2px;padding:3px 8px;font-size:12px}`

const viewerJS = `;
const list=document.getElementById('list'),img=document.getElementById('img'),cap=document.getElementById('cap');
let fi=0,zi=0,zoom=1;
function renderList(){list.innerHTML='';HOT.forEach((h,i)=>{const li=document.createElement('li');
li.textContent='#'+h.rank+' frame '+h.frame_index+' score '+h.score.toFixed(1);
li.onclick=()=>{fi=i;zi=0;renderList();show()};if(i===fi)li.className='sel';list.appendChild(li)})}
function show(){const h=HOT[fi],z=h.zooms[zi];
img.src=z?z.zoom_png:h.overlay_png;img.style.transform='scale('+zoom+')';img.style.transformOrigin='top left';
cap.textContent=z?('frame '+h.frame_index+' / '+z.label+' @ ('+z.region.x+','+z.region.y+')'):('frame '+h.frame_index+' overlay');}
img.onclick=()=>{zoom=zoom===1?2.5:1;show()};
document.addEventListener('keydown',e=>{if(e.key==='ArrowDown'){fi=(fi+1)%HOT.length;zi=0;zoom=1;renderList();show()}
if(e.key==='ArrowUp'){fi=(fi-1+HOT.length)%HOT.length;zi=0;zoom=1;renderList();show()}
if(e.key==='ArrowRight'){zi=(zi+1)%HOT[fi].zooms.length;zoom=1;show()}
if(e.key==='ArrowLeft'){zi=(zi-1+HOT[fi].zooms.length+HOT[fi].zooms.length)%HOT[fi].zooms.length;zoom=1;show()}});
renderList();show();`
