package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"rec2020-to-rec709-shoulder/internal/media"
	"rec2020-to-rec709-shoulder/internal/pipeline"
)

type contactSet struct {
	frame   int
	source  *pipeline.FloatImage
	outputs map[string]*pipeline.FloatImage
}

func renderSequence(o *options, jobs []frameJob, generated bool, decisions []pipeline.Decision) ([]pipeline.FrameMetrics, contactSet, error) {
	var metrics []pipeline.FrameMetrics
	contact := contactSet{frame: o.ContactFrame, outputs: map[string]*pipeline.FloatImage{}}
	variants := []string{pipeline.VariantHard, pipeline.VariantFixed, pipeline.VariantAdaptive}

	for _, job := range jobs {
		src, err := readJob(o, job, generated)
		if err != nil {
			return nil, contact, fmt.Errorf("read frame %d: %w", job.index, err)
		}
		if generated && o.SaveSource {
			sourcePath := filepath.Join(o.Out, "source_rec2020_pfm", fmt.Sprintf("frame_%04d.pfm", job.index))
			if err := media.WritePFM(sourcePath, src); err != nil {
				return nil, contact, err
			}
		}

		name := outputName(job, generated)
		for _, variant := range variants {
			params := pipeline.RenderParams{Variant: variant, Transfer: "rec709", PreserveHue: o.PreserveHue}
			switch variant {
			case pipeline.VariantFixed:
				params.Knee = o.FixedKnee
			case pipeline.VariantAdaptive:
				params.Knee = decisions[job.index].Knee
			}
			out := pipeline.RenderFrame(src, params)
			path := filepath.Join(o.Out, variantDirs[variant], name)
			if err := media.WritePNG(path, out, "rec709", o.Bit16); err != nil {
				return nil, contact, err
			}
			metrics = append(metrics, pipeline.CompareFrame(src, out, job.index, variant))
			if job.index == o.ContactFrame {
				contact.source = src
				contact.outputs[variant] = out
			}
		}
	}
	return metrics, contact, nil
}

func outputName(job frameJob, generated bool) string {
	if generated || job.path == "" {
		return fmt.Sprintf("frame_%04d.png", job.index)
	}
	base := filepath.Base(job.path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext) + ".png"
}

func writeVisuals(outDir string, contact contactSet) error {
	if contact.source == nil || len(contact.outputs) == 0 {
		return fmt.Errorf("contact frame %d was not rendered", contact.frame)
	}
	ordered := []*pipeline.FloatImage{
		contact.outputs[pipeline.VariantHard],
		contact.outputs[pipeline.VariantFixed],
		contact.outputs[pipeline.VariantAdaptive],
	}
	contactSheet := pipeline.HorizontalStrip(ordered, 6)
	if err := media.WritePNG(filepath.Join(outDir, "contact_compare.png"), contactSheet, "rec709", false); err != nil {
		return err
	}
	hueMap := pipeline.HueErrorStrip(contact.source, ordered, 30)
	if err := media.WritePNG(filepath.Join(outDir, "hue_shift_map.png"), hueMap, "linear", false); err != nil {
		return err
	}
	textureMap := pipeline.TextureStrip(ordered)
	if err := media.WritePNG(filepath.Join(outDir, "texture_map.png"), textureMap, "linear", false); err != nil {
		return err
	}
	clipMap := pipeline.ClipStrip(ordered)
	return media.WritePNG(filepath.Join(outDir, "clip_map.png"), clipMap, "linear", false)
}
